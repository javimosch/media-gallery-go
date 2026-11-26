#!/usr/bin/env python3
"""Compute face embeddings from saved face crops (no originals needed).

Usage: face_embed_crops.py FACES_DB STAGING_DB

Indexes the newest N faces (env N, default 2000) of FACES_DB into STAGING_DB.
FACES_DB is only read. Resumable. To index MORE faces than a previous run,
raise N: already-embedded rows get recomputed unless STAGING_DB is kept.

Run capped:  systemd-run --scope -p MemoryMax=1200M -p CPUQuota=150% nice -n 15 python3 face_embed_crops.py FACES_DB STAGING_DB
Apply to FACES_DB (back it up first):
  ATTACH '<STAGING_DB>' AS s;
  UPDATE faces SET embedding=(SELECT embedding FROM s.emb e WHERE e.id=faces.id)
   WHERE length(embedding)=0 AND EXISTS(SELECT 1 FROM s.emb e WHERE e.id=faces.id
     AND e.sha256=faces.sha256 AND e.face_idx=faces.face_idx AND length(e.embedding)=2048);
"""
import sys,time,random,os,sqlite3,numpy as np,cv2,onnxruntime as ort
from insightface.app import FaceAnalysis
if len(sys.argv)!=3: sys.exit('usage: face_embed_crops.py FACES_DB STAGING_DB')
faces_db,staging_db=sys.argv[1:]
so=ort.SessionOptions(); so.intra_op_num_threads=2; so.inter_op_num_threads=1; so.log_severity_level=3
so.add_session_config_entry('session.intra_op.allow_spinning','0')
cv2.setNumThreads(1)
_app=FaceAnalysis(name='buffalo_l',allowed_modules=['detection','recognition'],providers=['CPUExecutionProvider'],sess_options=so)
_app.prepare(ctx_id=-1,det_size=(320,320),det_thresh=0.5)
def mk(size,thr=0.5):
    class A:
        def get(self,im):
            _app.det_model.input_size=(size,size); _app.det_model.det_thresh=thr
            return _app.get(im)
    return A()
import sqlite3
def iou(a,b):
    x=max(0,min(a[2],b[2])-max(a[0],b[0])); y=max(0,min(a[3],b[3])-max(a[1],b[1])); i=x*y
    u=(a[2]-a[0])*(a[3]-a[1])+(b[2]-b[0])*(b[3]-b[1])-i
    return i/u if u>0 else 0
def expected(c):
    h,w=c.shape[:2]; fw=w/1.6; p=0.3*fw
    return (p,min(p,h*0.25),w-p,h-min(p,h*0.25))
apps={}
def detect(c,border,size,thr):
    a=apps.setdefault((size,thr),mk(size,thr))
    b=int(max(c.shape[:2])*border)
    im=cv2.copyMakeBorder(c,b,b,b,b,cv2.BORDER_CONSTANT) if b else c
    e=[v+b for v in expected(c)]
    fs=a.get(im)
    if not fs: return None,0,e,im
    best=max(fs,key=lambda f:iou(f.bbox,e)); return best,iou(best.bbox,e),e,im
def cascade(c):
    for border,size,thr in [(0.5,160,0.5),(1.0,160,0.5),(0.25,320,0.5),(0.5,160,0.3)]:
        x,s,_,_=detect(c,border,size,thr)
        if x is not None and s>=0.4: return x
    return None
# Resumable: writes only to the staging DB, never to FACES_DB.
N=int(os.environ.get('N','2000'))
db=sqlite3.connect('file:'+faces_db+'?mode=ro',uri=True)
rows=db.execute('select id,sha256,face_idx,thumb_path from faces order by id desc limit ?',(N,)).fetchall()
st=sqlite3.connect(staging_db)
st.execute('create table if not exists emb(id integer primary key,sha256 text,face_idx int,embedding blob)')
done={r[0] for r in st.execute('select id from emb')}
t=time.time(); n=ok=0
for fid,sha,idx,tp in rows:
    if fid in done: continue
    c=cv2.imread(tp) if tp else None
    x=cascade(c) if c is not None else None
    blob=b'' if x is None else np.asarray(x.normed_embedding,dtype='<f4').tobytes()
    st.execute('insert into emb values(?,?,?,?)',(fid,sha,idx,blob)); n+=1; ok+=bool(blob)
    if n%50==0: st.commit()
    if n%250==0: print('progress',n,'indexed',ok,round(n/(time.time()-t),2),'/s',flush=True)
st.commit()
print('DONE processed',n,'indexed',ok,'missed',n-ok,round(time.time()-t),'s',flush=True)
