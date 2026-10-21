
import fs from 'node:fs';
const js=fs.readFileSync('/mnt/d/dev/codebase/dev/timekeeper/web/timekeeper.js','utf8');
const start=js.indexOf('async function loadPluginStore');
const end=js.indexOf('async function loadPulse',start);
if(start===-1||end===-1) throw new Error('loader not found in source');
const loaderSrc=js.slice(start,end);

function makeEl(tag){
  return { tagName:tag, textContent:'', className:'', _attrs:{}, _children:[],
    get className(){return this._class;}, set className(v){this._class=v;},
    replaceChildren(){this._children=[];}, append(...c){this._children.push(...c);},
    setAttribute(k,v){this._attrs[k]=v;}, getAttribute(k){return this._attrs[k];} };
}
function makeDom(){
  const n={};
  for(const id of ['store-status','nav-store-count','store']) n[id]=makeEl('div');
  return n;
}
async function run(code,payload){
  const nodes=makeDom();
  const document={ querySelector:s=>nodes[s.replace('#','')],
                    createElement:t=>makeEl(t) };
  const fetchStub=async()=>({status:code,ok:code>=200&&code<300,json:async()=>payload});
  const fn=new Function('document','fetch',loaderSrc+'; return loadPluginStore;')(document,fetchStub);
  await fn();
  return nodes;
}
function itemCount(n){
  const first=n.store._children[0];
  return first ? (first._children||[]).length : 0;
}
(async()=>{
  const cases=[
    {label:'503 unconfigured (LIVE shape)',code:503,
     payload:{error:{code:'plugin_store_unconfigured',message:'The plugin store is not configured.'}},
     status:'Not configured',count:'',cls:'warning',items:null},
    {label:'200 zero plugins',code:200,
     payload:{schema_version:'timekeeper-plugin-index/v1',plugins:[]},
     status:'0 plugins',count:'',cls:'empty',items:null},
    {label:'200 one plugin (singular wording)',code:200,
     payload:{plugins:[{id:'solo.one',version:'1.0.0',name:'Solo',runtime:'node',protocol:'jsonrpc'}]},
     status:'1 plugin',count:'1',cls:'store-list',items:1},
    {label:'200 three plugins (full manifest)',code:200,
     payload:{plugins:[
       {id:'cactus.needle',version:'1.2.0',name:'Needle',runtime:'node',protocol:'jsonrpc',
        description:'demo plugin',capabilities:['fs.read','fs.write'],artifact:'needle.tgz',sha256:'a'.repeat(64)},
       {id:'other.one',version:'0.1.0',name:'One',runtime:'wasm',protocol:'http'},
       {id:'third.two',version:'2.0.0',name:'Two',runtime:'node',protocol:'jsonrpc'}]},
     status:'3 plugins',count:'3',cls:'store-list',items:3},
  ];
  let pass=0,fail=0;
  for(const c of cases){
    const n=await run(c.code,c.payload);
    const items=itemCount(n);
    const okStatus=n['store-status'].textContent===c.status;
    const okCount =n['nav-store-count'].textContent===c.count;
    const okClass =c.cls==='warning'?n.store.className.includes('warning')
                   :c.cls==='empty'  ?n.store.className==='empty'
                   :n.store.className==='store-list';
    const okItems =c.items===null?true:items===c.items;
    const noError =!n.store.className.includes('error');
    const ok=okStatus&&okCount&&okClass&&okItems&&noError;
    ok?pass++:fail++;
    console.log((ok?'PASS':'FAIL')+'  '+c.label+
      '  status="'+n['store-status'].textContent+'" count="'+n['nav-store-count'].textContent+
      '" items='+items+' class="'+n.store.className+'"');
    if(!ok) console.log('      want status='+c.status+' count='+c.count+' cls~'+c.cls+
                        ' items='+c.items+' noError=true');
  }
  console.log('\n'+pass+' passed, '+fail+' failed');
  process.exit(fail?1:0);
})();
