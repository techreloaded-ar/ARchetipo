(() => {
  'use strict';
  const $ = (selector, root = document) => root.querySelector(selector);
  const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];
  const key = 'archetipo-incremento-mockup-v1';
  const fresh = () => ({ published: false, merged: false, feedback: '', target: '', revised: false });
  let state;
  try { state = { ...fresh(), ...JSON.parse(sessionStorage.getItem(key) || '{}') }; } catch { state = fresh(); }
  const save = () => { try { sessionStorage.setItem(key, JSON.stringify(state)); } catch {} };
  const escape = value => String(value).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const version = () => state.revised ? 4 : 3;
  const storyCount = () => state.revised ? 4 : 3;
  const checks = () => state.revised ? 22 : 18;
  let toastTimer;
  function toast(message) {
    $('#toast').textContent = message;
    $('#toast').hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { $('#toast').hidden = true; }, 5000);
  }
  function closeDialogs() { $$('dialog[open]').forEach(dialog => dialog.close()); }
  function openDialog(id) {
    closeDialogs();
    const target = document.getElementById(id);
    if (!target) return;
    if (id === 'backlog-dialog') updateBacklog();
    if (id === 'pr-dialog') updatePR();
    target.showModal();
  }
  function updateBacklog() {
    const names = ['Esportare i clienti in Excel', 'Scegliere l’area commerciale', 'Riconoscere il file esportato'];
    if (state.revised) names.push('Affinare l’esportazione con il feedback');
    $('#backlog-content').innerHTML = state.published
      ? `<p class="muted" style="font-size:12px;line-height:1.8;margin-bottom:17px">${state.merged ? 'Il merge è confermato. Le storie dell’incremento sono consegnate.' : 'Le storie sono pubblicate come proposte, collegate alla PR #128. La consegna attende il merge.'}</p>${names.map((name, index) => `<article class="story-card" style="padding:14px"><div class="story-meta">US-${String(143 + index)} <span class="badge ${state.merged ? 'green' : 'indigo'}">${state.merged ? 'Consegnata' : 'Proposta · PR #128'}</span></div><h3 style="font-size:12px">${name}</h3></article>`).join('')}<p class="tiny muted">Le 12 storie già presenti restano nel backlog. Questa vista mostra l’incremento INC-024.</p>`
      : `<p class="muted" style="font-size:12px;line-height:1.8">Il backlog contiene 12 storie. Le ${storyCount()} storie di INC-024 sono ancora interne alla sandbox e compariranno qui solo all’apertura della PR.</p><div class="approval-recap"><div class="recap-line"><span>US-142 · Ricerca contatti per email</span><strong>Consegnata</strong></div><div class="recap-line"><span>US-141 · Ricerca contatti per nome</span><strong>Consegnata</strong></div></div><a class="btn" href="storie.html">Vedi le storie nella sandbox →</a>`;
  }
  function updatePR() {
    $('#pr-content').innerHTML = `<p class="muted" style="font-size:12px;line-height:1.8">${state.merged ? 'PR integrata. Il risultato approvato è entrato nel prodotto.' : 'PR aperta per la revisione tecnica. Il prodotto in uso non è ancora stato aggiornato.'}</p><div class="approval-recap"><h3>Esportazione clienti</h3><div class="recap-line"><span>Stato</span><strong>${state.merged ? 'Merged' : 'In revisione'}</strong></div><div class="recap-line"><span>Versione approvata</span><strong>${version()}</strong></div><div class="recap-line"><span>Storie collegate</span><strong>${storyCount()}</strong></div><div class="recap-line"><span>Controlli</span><strong>${checks()} superati</strong></div></div><p class="tiny muted">Questa PR è simulata. Nessuna operazione viene eseguita su GitHub.</p>`;
    $('[data-action="simulate-merge"]').hidden = state.merged;
  }
  function sync() {
    const banner = $('#state-banner');
    banner.hidden = !state.published;
    if (state.published) {
      banner.textContent = state.merged
        ? 'Merge confermato · Incremento integrato. Le storie sono consegnate nel backlog. La cronologia e le approvazioni rimangono consultabili.'
        : `PR #128 aperta · ${storyCount()} storie pubblicate come proposte. Il risultato è in revisione tecnica e attende il merge.`;
      $('[data-main-status]').className = `badge ${state.merged ? 'green' : 'indigo'}`;
      $('[data-main-status]').textContent = state.merged ? 'Integrato nel prodotto' : 'In revisione · PR #128';
      $('[data-rail-state]').textContent = state.merged ? 'Integrato' : 'PR aperta · in revisione';
      $('[data-review-step]').className = 'step done';
      $('[data-pr-step]').className = state.merged ? 'step done' : 'step current';
      if (state.merged) $('[data-merge-step]').className = 'step current';
      $$('[data-action="approve"]').forEach(button => { button.innerHTML = 'Vedi la PR #128 ↗'; });
      const scope = $('[data-story-scope]');
      if (scope) scope.textContent = state.merged ? 'Consegnate nel backlog' : 'Proposte nel backlog';
      if ($('#feedback')) {
        $('#feedback').disabled = true;
        $('#feedback').placeholder = state.merged ? 'Questo incremento è integrato.' : 'Versione approvata. I nuovi cambiamenti passano dalla revisione della PR.';
        $('#feedback-form button[type="submit"]').disabled = true;
        $$('.feedback-suggestions button').forEach(button => { button.disabled = true; });
      }
    }
    if (state.revised) {
      $$('[data-count]').forEach(count => { count.textContent = '4'; });
      if (!state.published) $('[data-rail-state]').textContent = 'Da provare · giro 4';
      const panelBadge = $('.panel-header .badge');
      if (panelBadge) panelBadge.textContent = 'Giro 4';
      const bottom = $('.preview-bottom>span');
      if (bottom) bottom.innerHTML = '<i class="dot" style="color:#5b8562"></i>Anteprima pronta · versione 4';
      const testLabel = $('.test-summary>span:last-child');
      if (testLabel) testLabel.textContent = '22 controlli · nessun blocco';
      if ($('.change-card')) {
        $('.change-card h2').textContent = 'Cosa è cambiato nel giro 4';
        $('.changes').innerHTML = `<div class="change"><div><strong>Feedback concordato</strong><p>${escape(state.feedback)}</p></div></div><div class="change"><div><strong>${escape(state.target)}</strong><p>Una storia aggiunta, intento aggiornato.</p></div></div>`;
      }
      if ($('#chat-log') && !$('#feedback-complete')) {
        const el = document.createElement('div');
        el.id = 'feedback-complete';
        el.innerHTML = `<div class="human-msg">${escape(state.feedback)}</div><div class="chat-person"><span class="agent-mark">✳</span>ARchetipo</div><p class="chat-msg">Il giro 4 è pronto. L’intento è aggiornato e il feedback è collegato alla quarta storia. Puoi provare il risultato.</p>`;
        $('#chat-log').append(el);
        $('#chat-log').scrollTop = $('#chat-log').scrollHeight;
      }
      if (/fatturato|ricav|valore/i.test(state.feedback) && !$('.revenue-head') && $('.client-table')) {
        const th = document.createElement('th'); th.textContent = 'Fatturato'; th.className = 'revenue-head';
        $('.client-table thead tr').append(th);
        const values = ['128.400 €', '96.250 €', '54.000 €', '82.700 €'];
        if (state.target !== 'Solo nel file Excel') {
          $$('.client-table tbody tr').forEach((row, index) => { const td = document.createElement('td'); td.textContent = values[index]; row.append(td); });
          $('.client-table th:first-child').style.width = '33%';
        } else th.remove();
      }
      if (document.body.dataset.page === 'storie' && !$('#feedback-story')) {
        const article = document.createElement('article'); article.className = 'story-card'; article.id = 'feedback-story';
        article.innerHTML = `<div class="story-meta">INC-024 / S-04 · Versione 1 <span class="badge green">Verificata nella sandbox</span></div><h3>Affinare l’esportazione con il feedback</h3><p>${escape(state.feedback)}</p><div class="revised-note">Aggiunta al giro 4 · ${escape(state.target)}</div><div class="story-bottom"><span>✓ Criteri verificati</span><span>4 controlli superati</span></div>`;
        $('.page-grid>section').append(article);
        $('.story-intro+p')?.remove();
      }
      if ($('#extra-iteration')) $('#extra-iteration').innerHTML = `<article class="iteration latest"><div class="iteration-head"><span class="iteration-num">04</span><div><h3>Il feedback diventa un nuovo giro</h3><span class="badge peach">${state.published ? 'Approvato' : 'Da provare'}</span></div><time>Adesso</time></div><p class="iteration-feedback">${escape(state.feedback)}</p><ul><li>Ambito concordato: ${escape(state.target)}.</li><li>Intento aggiornato alla versione 4.</li><li>Quarta storia collegata al feedback.</li></ul><div class="iteration-foot"><span class="badge green">22 verifiche superate</span><a class="text-link" href="index.html">Prova questa versione →</a></div></article>`;
      if (document.body.dataset.page === 'intento' && !$('#feedback-intent')) {
        $('.paper-top .badge').textContent = '✓ Approvato · v4';
        const note = document.createElement('section'); note.className = 'intent-section'; note.id = 'feedback-intent';
        note.innerHTML = `<h3>Concordato nel giro 4</h3><p>${escape(state.feedback)} Ambito: ${escape(state.target)}.</p>`;
        $('.paper-body').append(note);
      }
    }
    $('[data-approve-version]').textContent = `Versione ${version()} · ${checks()} verifiche superate`;
    $('[data-approve-intent]').textContent = `v${version()} · approvato`;
    $('[data-publish-story-count]').textContent = `${storyCount()} · come proposte`;
  }
  document.addEventListener('click', event => {
    const dialogButton = event.target.closest('[data-dialog]');
    if (dialogButton) { openDialog(dialogButton.dataset.dialog); return; }
    if (event.target.closest('[data-close]')) { event.target.closest('dialog').close(); return; }
    const suggestion = event.target.closest('[data-suggestion]');
    if (suggestion && !state.published) { $('#feedback').value = suggestion.dataset.suggestion; $('#feedback').focus(); return; }
    const action = event.target.closest('[data-action]')?.dataset.action;
    if (!action) return;
    if (action === 'approve') {
      if (state.published) { openDialog('pr-dialog'); return; }
      $('#approval-check').checked = false;
      $('#publish-pr').disabled = true;
      openDialog('approval-dialog');
    }
    if (action === 'refresh-preview') { $('#client-search').value = ''; $('#area-filter').value = ''; filterClients(); toast('Anteprima aggiornata. I dati di prova sono ripristinati.'); }
    if (action === 'expand-preview') {
      const card = $('#preview-card');
      if (!document.fullscreenElement && card.requestFullscreen) card.requestFullscreen().catch(() => toast('Anteprima già disponibile in questa schermata.'));
      else if (document.fullscreenElement) document.exitFullscreen();
      else toast('Puoi ingrandire la finestra del browser per esplorare l’anteprima.');
    }
    if (action === 'export') {
      const rows = $$('.client-table tbody tr').filter(row => !row.hidden);
      if (!rows.length) { toast('Nessun cliente corrisponde al filtro. Prova un’altra ricerca.'); return; }
      const area = ($('#area-filter').value || 'tutte-le-aree').toLowerCase().replaceAll(' ', '-');
      toast(`Esportazione simulata: clienti_${area}_15-10.xlsx · ${rows.length} ${rows.length === 1 ? 'cliente' : 'clienti'}${state.revised && /fatturato|ricav|valore/i.test(state.feedback) ? ' · include il fatturato' : ''}`);
    }
    if (action === 'revise-intent') {
      try { sessionStorage.setItem('incremento-revise-intent', 'Vorrei rivedere l’intento di questo incremento: '); } catch {}
      location.href = 'index.html#feedback';
    }
    if (action === 'simulate-merge') { state.published = true; state.merged = true; save(); closeDialogs(); sync(); toast('Merge simulato. Storie consegnate, incremento integrato.'); }
    if (action === 'reset-demo') { state = fresh(); save(); location.href = 'index.html'; }
    if (action === 'confirm-feedback') {
      state.revised = true; save();
      $('#feedback-plan')?.remove();
      sync();
      if ($('#feedback')) $('#feedback').value = '';
      toast('Giro 4 simulato: intento, storie e risultato aggiornati.');
    }
  });
  $$('dialog').forEach(dialog => dialog.addEventListener('click', event => {
    if (event.target !== dialog) return;
    const rect = dialog.getBoundingClientRect();
    if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) dialog.close();
  }));
  $('#approval-check').addEventListener('change', event => { $('#publish-pr').disabled = !event.target.checked; });
  $('#publish-pr').addEventListener('click', () => {
    if (!$('#approval-check').checked) return;
    state.published = true; save(); sync(); openDialog('success-dialog');
  });
  function filterClients() {
    const search = $('#client-search')?.value.toLowerCase().trim() || '';
    const area = $('#area-filter')?.value || '';
    const rows = $$('.client-table tbody tr');
    rows.forEach(row => { row.hidden = Boolean((area && row.dataset.area !== area) || (search && !row.dataset.client.includes(search))); });
    const count = rows.filter(row => !row.hidden).length;
    if ($('#client-count')) $('#client-count').textContent = `${count} ${count === 1 ? 'cliente' : 'clienti'} · dati dimostrativi`;
  }
  $('#client-search')?.addEventListener('input', filterClients);
  $('#area-filter')?.addEventListener('change', filterClients);
  $('#feedback-form')?.addEventListener('submit', event => {
    event.preventDefault();
    const feedback = $('#feedback').value.trim();
    if (!feedback || state.published) { $('#feedback').focus(); return; }
    if (state.revised) { toast('Questo prototipo mostra un giro aggiuntivo. Usa “Ricomincia la demo” nella guida per riprovarlo.'); return; }
    state.feedback = feedback;
    $('#feedback-plan')?.remove();
    const plan = document.createElement('div'); plan.className = 'feedback-plan'; plan.id = 'feedback-plan';
    plan.innerHTML = `<div class="human-msg" style="margin-top:0">${escape(feedback)}</div><strong>Chiarisco un punto prima di ripartire.</strong><p style="margin-top:7px">Questo cambiamento deve comparire solo nel file esportato o anche nell’elenco clienti?</p><div class="discovery-prompts"><button type="button" data-target="Solo nel file Excel">Solo nel file</button><button type="button" data-target="Nel file e nell’elenco clienti">In entrambi</button></div><div id="feedback-confirmation" hidden></div>`;
    $('#chat-log').append(plan);
    $('#chat-log').scrollTop = $('#chat-log').scrollHeight;
    $('#feedback').value = '';
    plan.addEventListener('click', event => {
      const target = event.target.closest('[data-target]');
      if (!target) return;
      state.target = target.dataset.target;
      $$('.discovery-prompts button', plan).forEach(button => { button.style.background = button === target ? '#e5e3f4' : ''; });
      const confirmation = $('#feedback-confirmation'); confirmation.hidden = false;
      confirmation.innerHTML = `<p style="margin-top:12px">D’accordo: <strong>${escape(state.target.toLowerCase())}</strong>. Aggiornerò l’intento e aggiungerò una storia collegata a questo feedback. Le altre tre restano nello stesso incremento.</p><button type="button" class="btn primary small" data-action="confirm-feedback">Conferma e avvia il giro 4 →</button>`;
      $('#chat-log').scrollTop = $('#chat-log').scrollHeight;
    });
  });
  $('#new-form').addEventListener('submit', event => {
    event.preventDefault();
    const title = $('#new-name').value.trim();
    const intent = $('#new-intent').value.trim();
    if (!title || !intent) return;
    $('#new-form').hidden = true;
    const interview = $('#new-interview'); interview.hidden = false;
    interview.innerHTML = `<div class="human-msg"><strong>${escape(title)}</strong><br>${escape(intent)}</div><div class="new-discussion"><div class="chat-person"><span class="agent-mark">✳</span>ARchetipo</div><h3>Partiamo da chi userà questo risultato.</h3><p>Chi incontra questo problema e quale esempio concreto ci permetterà di capire che lo abbiamo risolto?</p><form id="discovery-form"><label for="discovery-answer" class="eyebrow" style="display:block;margin-top:12px">Persone e risultato atteso</label><textarea id="discovery-answer" required placeholder="Es. I responsabili commerciali: ogni lunedì…"></textarea><button class="btn primary" type="submit">Continua →</button></form></div>`;
    $('#discovery-form').addEventListener('submit', event => {
      event.preventDefault();
      const answer = $('#discovery-answer').value.trim(); if (!answer) return;
      interview.innerHTML = `<div class="approval-recap"><span class="eyebrow">Intento da confermare</span><h3 style="margin-top:9px">${escape(title)}</h3><p class="muted" style="font-size:12px;line-height:1.8">${escape(intent)}</p><p style="margin-top:12px;font-size:12px;line-height:1.8"><strong>Per chi e come verificarlo</strong><br>${escape(answer)}</p></div><p class="muted" style="font-size:11px;line-height:1.8">Nel prodotto reale continueremmo a chiarire ambito e vincoli prima dello sviluppo. Qui puoi vedere il passaggio di conferma.</p><button class="btn primary" id="approve-new" style="width:100%;margin-top:16px">Conferma questo intento</button>`;
      $('#approve-new').addEventListener('click', () => { interview.innerHTML = `<div class="success-icon">✓</div><h3>Intento confermato</h3><p class="muted" style="font-size:12px;line-height:1.8;margin-top:12px">Il passo successivo è derivare le storie e avviare lo sviluppo nella sandbox dedicata. Per esplorare quei passaggi, torna all’esempio “Esportazione clienti”.</p><button class="btn primary" style="margin-top:18px" data-close>Torna all’esempio completo</button>`; });
    });
  });
  sync();
  try {
    const revise = sessionStorage.getItem('incremento-revise-intent');
    if (revise && $('#feedback') && !state.published) { $('#feedback').value = revise; $('#feedback').focus(); sessionStorage.removeItem('incremento-revise-intent'); }
  } catch {}
})();
