package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const Version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "start":
		handleStart()
	case "stop":
		handleStop()
	case "status":
		handleStatus()
	case "passwd":
		if err := handlePasswd(os.Args[2:], os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "version":
		fmt.Printf("media-gallery-go v%s\n", Version)
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printHelp()
		os.Exit(1)
	}
}

func handleStart() {
	cmd := flag.NewFlagSet("start", flag.ExitOnError)
	port := cmd.Int("port", 8090, "Port for HTTP server")
	daemon := cmd.Bool("daemon", false, "Run as daemon in background")
	mediaRoot := cmd.String("media", "./media", "Media root directory")
	dbPath := cmd.String("db", "./inventory.db", "SQLite inventory database path")
	thumbDir := cmd.String("thumbs", "./thumbs", "Thumbnail cache directory")
	authUser := cmd.String("user", "", "Basic auth username")
	authPass := cmd.String("pass", "", "Basic auth password")
	maxConcurrent := cmd.Int("max-dl", 3, "Max concurrent downloads/streams")
	facesDB := cmd.String("faces-db", "", "Face recognition SQLite DB (opt-in, empty = disabled)")
	hdSince := cmd.String("hd-since", "", "HD thumbnails for media dated YYYY-MM or later")
	hdList := cmd.String("hd-folders", "", "Comma-separated folders that get HD thumbnails for every month")
	authPath := cmd.String("auth-file", "", "Require sign-in, using this private users file (see the passwd command)")
	trustedProxies := cmd.String("trusted-proxies", "", "Proxy addresses allowed to name the visitor in X-Forwarded-For")
	replicaFrom := cmd.String("replica-from", "", "Addresses that may register replicas without signing in")
	groups := cmd.String("groups", "", "Folder groups, e.g. \"Cameras=cam-*;Other=*\"")
	cmd.Parse(os.Args[2:])

	if err := configureGroups(*groups); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := configureHDThumbs(*hdSince, *hdList); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	// validate media root exists
	if _, err := os.Stat(*mediaRoot); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Media root not found: %s\n", *mediaRoot)
		os.Exit(90)
	}
	// validate db exists
	if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Database not found: %s\n", *dbPath)
		os.Exit(90)
	}

	cfg := ServerConfig{
		Port:          *port,
		MediaRoot:     *mediaRoot,
		DBPath:        *dbPath,
		ThumbDir:      *thumbDir,
		AuthUser:      *authUser,
		AuthPass:      *authPass,
		MaxConcurrent: *maxConcurrent,
		FacesDB:       *facesDB,
		AuthFile:       *authPath,
		TrustedProxies: *trustedProxies,
		ReplicaFrom:    *replicaFrom,
		Groups:        *groups,
		HDSince:       *hdSince,
		HDFolders:     *hdList,
	}

	if *daemon {
		startDaemon(cfg)
	} else {
		startServer(cfg)
	}
}

func handleStop() {
	stopDaemon()
}

func handleStatus() {
	checkDaemonStatus()
}

func printHelp() {
	fmt.Println("media-gallery-go - High-performance media gallery web UI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  media-gallery-go <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  start       Start HTTP server with web UI")
	fmt.Println("  stop        Stop daemon server")
	fmt.Println("  status      Check daemon status")
	fmt.Println("  passwd      Set a user's password in an auth file (read from stdin)")
	fmt.Println("  version     Show version information")
	fmt.Println("  help        Show this help message")
	fmt.Println()
	fmt.Println("Start Options:")
	fmt.Println("  -port int          Port for HTTP server (default 8090)")
	fmt.Println("  -daemon            Run as daemon in background")
	fmt.Println("  -media string      Media root directory (default ./media)")
	fmt.Println("  -db string         SQLite inventory database (default ./inventory.db)")
	fmt.Println("  -thumbs string     Thumbnail cache directory (default ./thumbs)")
	fmt.Println("  -user string       Basic auth username (empty = no auth)")
	fmt.Println("  -pass string       Basic auth password")
	fmt.Println("  -max-dl int        Max concurrent downloads/streams (default 3)")
	fmt.Println("  -auth-file string  Require sign-in with a login page (default off)")
	fmt.Println("  -trusted-proxies   Proxies allowed to set X-Forwarded-For")
	fmt.Println("  -replica-from      Addresses that may register replicas without sign-in")
	fmt.Println("  -groups string     Folder groups, e.g. \"Cameras=cam-*;Other=*\" (default none)")
	fmt.Println("  -hd-since string   HD thumbnails for media dated YYYY-MM or later, or \"all\" (default off)")
	fmt.Println("  -hd-folders string Comma-separated folders with HD thumbnails for every month")
	fmt.Println()
	fmt.Println("API Endpoints:")
	fmt.Println("  GET /api/browse       Browse files (paginated)")
	fmt.Println("  GET /api/search?q=    Search files by path")
	fmt.Println("  GET /api/dupes        List duplicate groups")
	fmt.Println("  GET /api/dupes/:sha   Files in a dupe group")
	fmt.Println("  GET /api/stats        Collection statistics")
	fmt.Println("  GET /api/categories   List categories")
	fmt.Println("  GET /api/yearmonths   List year-month folders")
	fmt.Println("  GET /api/file/:sha    File metadata")
	fmt.Println("  GET /api/thumb/:sha   Thumbnail (auto-generated, cached)")
	fmt.Println("  GET /api/download/:sha  Download original file")
	fmt.Println("  GET /api/stream/:sha    Stream video with Range support")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  media-gallery-go start -media ./media -db ./inventory.db")
	fmt.Println("  media-gallery-go start -port 3000 -user admin -pass secret")
	fmt.Println("  media-gallery-go start -daemon")
}

func absPath(p string) string {
	a, _ := filepath.Abs(p)
	return a
}
