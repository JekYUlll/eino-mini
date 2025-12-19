// Single, clean frontend controller for Eino
const STORAGE_KEY = 'eino_conversations_v1';
const API_BASE = window.API_BASE || 'http://localhost:8080';

function uid(){return Math.random().toString(36).slice(2,9)}
function now(){return new Date().toLocaleString()}

let conversations = []
let currentId = null

let $convs, $messages, $messagesHeader, $prompt, $sendBtn, $newConvBtn, $clearBtn, $convSearch

function load(){
  try{conversations = JSON.parse(localStorage.getItem(STORAGE_KEY)) || []}catch(e){conversations=[]}
  if(conversations.length>0) currentId = conversations[0].id
}

function save(){localStorage.setItem(STORAGE_KEY, JSON.stringify(conversations))}

function createConversation(title){
  const c = {id:uid(), title: title||'新对话', created: Date.now(), messages:[]}
  conversations.unshift(c)
  currentId = c.id
  save(); renderSidebar(); renderMessages()
}

function deleteConversation(id){
  conversations = conversations.filter(c=>c.id!==id)
  if(currentId===id) currentId = conversations.length?conversations[0].id:null
  save(); renderSidebar(); renderMessages()
}

function renderSidebar(){
  if(!$convs) return
  const q = ($convSearch && $convSearch.value || '').toLowerCase()
  $convs.innerHTML=''
  conversations.filter(c=>c.title.toLowerCase().includes(q)).forEach(c=>{
    const el = document.createElement('div')
    el.className='conv-item'+(c.id===currentId?' active':'')
    el.innerHTML = `<div class="conv-left" onclick="selectConversation('${c.id}')"><strong>${escapeHtml(c.title)}</strong><small>${new Date(c.created).toLocaleString()}</small></div><div class="conv-right"><button title="重命名" data-rename="${c.id}">✎</button><button title="删除" data-id="${c.id}">✕</button></div>`
    $convs.appendChild(el)
  })
  $convs.querySelectorAll('button[data-id]').forEach(b=>b.addEventListener('click', e=>{deleteConversation(e.currentTarget.dataset.id)}))
  $convs.querySelectorAll('button[data-rename]').forEach(b=>b.addEventListener('click', e=>{renameConversation(e.currentTarget.dataset.rename)}))
}

function renameConversation(id){
  const conv = conversations.find(c=>c.id===id)
  if(!conv) return
  const newName = prompt('为对话输入新名称：', conv.title || '')
  if(newName===null) return // cancel
  const name = (newName||'').trim()
  if(name.length===0) return
  conv.title = name
  save(); renderSidebar(); renderMessages()
}

window.selectConversation = function(id){ currentId=id; renderSidebar(); renderMessages() }

function renderMessages(){
  const conv = conversations.find(c=>c.id===currentId)
  $messages.innerHTML = ''
  if(!conv){
    $messagesHeader.innerHTML = `<div class="mh-left"><div class="mh-avatar"><img src="assets/logo.svg" class="mh-avatar-img"></div><div class="mh-text"><div class="mh-title">选择或新建一个对话</div><div class="mh-sub">暂无对话</div></div></div><div class="mh-actions"></div>`
    return
  }

  // Populate rich header
    $messagesHeader.innerHTML = `
    <div class="mh-left">
      <div class="mh-avatar"><div class="mh-avatar-initial">E</div></div>
      <div class="mh-text">
        <div class="mh-title">${escapeHtml(conv.title)}</div>
        <div class="mh-sub">创建于 ${new Date(conv.created).toLocaleString()}</div>
      </div>
    </div>
    <div class="mh-actions">
      <button id="renameConvBtn" title="重命名对话">✎</button>
      <button id="clearConvBtn" title="清空当前对话">🗑</button>
    </div>
  `

  // bind header action buttons (overwrite handlers to avoid duplicates)
  const rb = document.getElementById('renameConvBtn')
  if(rb) rb.onclick = ()=>{ renameConversation(conv.id) }
  const cb = document.getElementById('clearConvBtn')
  if(cb) cb.onclick = ()=>{
    const ok = confirm('确认清空此对话？此操作不可撤销。')
    if(!ok) return
    conv.messages = []
    save(); renderMessages()
  }
  conv.messages.forEach(m=>{
    const d = document.createElement('div')
    const cls = 'msg '+(m.role==='user'?'user':'assistant') + (m._typing? ' typing':'' )
    d.className = cls
    if(m._typing){
      d.innerHTML = `<div class="meta"><strong>Eino</strong> <span class="time">${m.time||''}</span></div><div class="content markdown-content"><div class="dots"><span></span><span></span><span></span></div></div>`
      $messages.appendChild(d)
      return
    }
    let contentHtml = ''
    if(m.role === 'assistant' && typeof marked !== 'undefined'){
      try{
        const raw = marked.parse(m.content || '')
        contentHtml = (typeof DOMPurify !== 'undefined') ? DOMPurify.sanitize(raw) : raw
      }catch(e){
        contentHtml = escapeHtml(m.content)
      }
    }else{
      contentHtml = escapeHtml(m.content)
    }
    d.innerHTML = `<div class="meta"><strong>${m.role==='user'?'你':'Eino'}</strong> <span class="time">${m.time||''}</span></div><div class="content markdown-content">${contentHtml}</div>`
    $messages.appendChild(d)
  })
  $messages.scrollTop = $messages.scrollHeight
  setTimeout(()=>{
    document.querySelectorAll('.markdown-content pre').forEach(pre=>{
      if(pre.querySelector('.code-copy-btn')) return
      const btn = document.createElement('button')
      btn.className = 'code-copy-btn'
      btn.textContent = '复制'
      btn.addEventListener('click', async ()=>{
        const code = pre.querySelector('code') ? pre.querySelector('code').innerText : pre.innerText
        try{ await navigator.clipboard.writeText(code); btn.textContent='已复制'; setTimeout(()=>btn.textContent='复制',1200)}catch(e){btn.textContent='复制失败';setTimeout(()=>btn.textContent='复制',1200)}
      })
      pre.style.position='relative'
      pre.appendChild(btn)
    })
  },50)
}

function escapeHtml(s){return (s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>')}

async function sendCurrent(){
  const conv = conversations.find(c=>c.id===currentId)
  if(!conv) { createConversation('对话'); return }
  const text = $prompt.value.trim()
  if(!text) return
  const userMsg = {role:'user', content:text, time: now()}
  conv.messages.push(userMsg); save(); renderMessages(); $prompt.value='';

  // If conversation still has default title, derive a title from user's first message
  try{
    const defaultNames = ['新对话','对话']
    if(defaultNames.includes((conv.title||'').trim())){
      const firstLine = text.split('\n')[0].trim()
      const snippet = firstLine.length>40? firstLine.slice(0,40).trim() + '...' : firstLine || '对话'
      conv.title = snippet
      save(); renderSidebar()
    }
  }catch(e){/* ignore */}

  // send to backend /ask with typing indicator
  const placeholder = {role:'assistant', content: '', time: now(), _typing: true}
  conv.messages.push(placeholder); renderMessages()
  $sendBtn.disabled = true
  try{
    const payload = {question: text}
    const res = await fetch(API_BASE + '/ask', {method:'POST',headers:{'Content-Type':'application/json'}, body:JSON.stringify(payload)})
    if(!res.ok) throw new Error('请求失败 '+res.status)
    const data = await res.json()
    const reply = data.answer || data.reply || data.text || JSON.stringify(data)
    const idx = conv.messages.findIndex(m=>m._typing)
    if(idx>=0) conv.messages.splice(idx,1)
    const assistantMsg = {role:'assistant', content: reply, time: now()}
    conv.messages.push(assistantMsg)
    save(); renderMessages()
  }catch(err){
    const idx = conv.messages.findIndex(m=>m._typing)
    if(idx>=0) conv.messages.splice(idx,1)
    const errMsg = {role:'assistant', content: '请求出错：'+err.message, time: now()}
    conv.messages.push(errMsg); save(); renderMessages()
  } finally {
    $sendBtn.disabled = false
  }
}

document.addEventListener('DOMContentLoaded', ()=>{
  // bind DOM elements after load to avoid nulls
  $convs = document.getElementById('conversations')
  $messages = document.getElementById('messages')
  $messagesHeader = document.getElementById('messagesHeader')
  $prompt = document.getElementById('prompt')
  $sendBtn = document.getElementById('sendBtn')
  $newConvBtn = document.getElementById('newConvBtn')
  $clearBtn = document.getElementById('clearBtn')
  $convSearch = document.getElementById('convSearch')

  console.log('Eino frontend: DOM bound', {newConvBtn: !!$newConvBtn, sendBtn: !!$sendBtn})

  load(); renderSidebar(); renderMessages();
  if($newConvBtn) $newConvBtn.addEventListener('click', ()=>createConversation('新对话'))
  if($sendBtn) $sendBtn.addEventListener('click', sendCurrent)
  if($clearBtn) $clearBtn.addEventListener('click', ()=>{
    const conv = conversations.find(c=>c.id===currentId);
    if(!conv) return;
    const ok = confirm('确认清空当前对话？此操作不可撤销。')
    if(!ok) return;
    conv.messages = [];
    save(); renderMessages()
  })
  if($convSearch) $convSearch.addEventListener('input', ()=>renderSidebar())
  const form = document.getElementById('inputForm')
  if(form) form.addEventListener('submit', e=>{ e.preventDefault(); sendCurrent()})
  if($prompt) $prompt.addEventListener('keydown', e=>{
    if((e.ctrlKey||e.metaKey) && e.key==='Enter'){ e.preventDefault(); sendCurrent() }
  })
})
