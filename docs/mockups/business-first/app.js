'use strict';
// Solo navigazione del mockup; nessuna chiamata ad ARchetipo e nessuna scrittura.
const data = window.mockupData;
const specs = new Map(data.specs.map(spec => [spec.code, spec]));
let selectedSpec = null;
let conversationSpec = null;
let conversationOpen = false;
let currentView = 'conversation';
let noticeTimer;
const byId = id => document.getElementById(id);
let selectedActivity = null;
// Le associazioni tecniche restano interne. Nel prodotto arrivano dal runtime
// e dalle azioni dichiarate dal server; qui descrivono solo il mockup.
const activityEntries = {
  product: {title:'Definisci un prodotto', description:'Chiarisci obiettivi, utenti e perimetro.', output:'Documento di prodotto', label:'Descrivi il prodotto', placeholder:'Descrivi il prodotto che vuoi definire…', skill:'archetipo-inception'},
  story: {title:'Nuova storia', description:'Descrivi il bisogno, rispondi alle domande e rivedi la proposta.', output:'Storia per il backlog', label:'Descrivi il bisogno', placeholder:'Quale bisogno deve risolvere la nuova storia?', skill:'archetipo-spec'},
  design: {title:'Nuovo mockup', description:'Indica la schermata o il flusso che vuoi rappresentare.', output:'Prototipo consultabile', label:'Descrivi la schermata o il flusso', placeholder:'Quale schermata o flusso vuoi rappresentare?', skill:'archetipo-design'},
  free: {title:'Conversazione libera', description:'Fai domande sul progetto o su una storia.', output:'Conversazione', label:'Messaggio', placeholder:'Scrivi all’agente…', skill:null}
};
function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}
function notice(text) {
  byId('notice').textContent = text;
  byId('notice').hidden = false;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => { byId('notice').hidden = true; }, 4500);
}
function showView(view) {
  currentView = view;
  for (const name of ['conversation', 'board', 'spec']) byId(name + '-view').hidden = name !== view;
  document.querySelectorAll('[data-view]').forEach(button => {
    const active = button.dataset.view === view;
    button.classList.toggle('active', active);
    if (active) button.setAttribute('aria-current', 'page');
    else button.removeAttribute('aria-current');
  });
  const url = new URL(location.href);
  url.searchParams.set('view', view === 'spec' ? 'board' : view);
  history.replaceState(null, '', url);
}
function previewAction(code, action) {
  notice('Mockup: ' + action.label + ' su ' + code + '. Nessuna esecuzione avviata.');
}
function actionButtons(spec) {
  const container = element('div', 'card-actions');
  spec.actions.forEach(action => {
    const button = element('button', 'primary', action.label);
    button.type = 'button';
    button.setAttribute('aria-label', action.label + ' ' + spec.code);
    button.addEventListener('click', () => previewAction(spec.code, action));
    container.append(button);
  });
  const discuss = element('button', 'text-button', 'Conversa');
  discuss.type = 'button';
  discuss.setAttribute('aria-label', 'Conversa su ' + spec.code);
  discuss.addEventListener('click', () => openConversation(spec.code));
  container.append(discuss);
  return container;
}
function renderBoard() {
  byId('source').textContent = data.source;
  const done = data.specs.filter(spec => spec.status === 'DONE').length;
  const inProgress = data.specs.filter(spec => spec.status === 'IN PROGRESS').length;
  byId('board-count').textContent = data.specs.length + ' storie · ' + inProgress + ' in corso · ' + done + ' concluse';
  data.statuses.forEach(status => {
    const members = data.specs.filter(spec => spec.status === status);
    const column = element('section', 'column');
    column.dataset.status = status;
    column.setAttribute('aria-label', status);
    const heading = element('header', 'column-header');
    heading.append(element('span', 'column-dot'), element('span', '', status), element('span', 'column-count', members.length));
    column.append(heading);
    members.forEach(spec => {
      const card = element('article', 'card');
      const meta = element('div', 'card-meta');
      meta.append(element('span', '', spec.code), element('span', 'priority', spec.priority));
      const title = element('button', 'card-title', spec.title);
      title.type = 'button';
      title.setAttribute('aria-label', 'Apri ' + spec.code + ': ' + spec.title);
      title.addEventListener('click', () => openSpec(spec.code));
      const bottom = element('div', 'card-bottom');
      bottom.append(element('span', '', spec.epic.code), element('span', '', spec.points + ' pt'));
      card.append(meta, title, bottom, actionButtons(spec));
      column.append(card);
    });
    if (!members.length) column.append(element('div', 'empty-column', 'Nessuna spec'));
    byId('board-columns').append(column);
  });
}
function renderText(text, container) {
  // Mantiene il testo della fonte: visualizza titoli e paragrafi senza riscriverli.
  container.replaceChildren();
  const blocks = text.split(/\n\s*\n/);
  for (const block of blocks) {
    if (/^#{1,6} /.test(block) && !block.includes('\n')) {
      container.append(element('h2', '', block.replace(/^#{1,6} /, '')));
    } else {
      container.append(element('p', '', block));
    }
  }
}
function showDocument(kind) {
  document.querySelectorAll('[data-document]').forEach(button => {
    const active = button.dataset.document === kind;
    button.classList.toggle('active', active);
    button.setAttribute('aria-pressed', String(active));
  });
  const spec = specs.get(selectedSpec);
  const text = kind === 'story' ? spec.body : spec.plan?.body || 'Nessun piano salvato.';
  renderText(text, byId('spec-document'));
}
function openSpec(code) {
  const spec = specs.get(code);
  selectedSpec = code;
  byId('spec-title').textContent = spec.title;
  byId('spec-meta').replaceChildren(
    element('span', '', spec.code),
    element('span', 'status', spec.status),
    element('span', '', spec.epic.code),
    element('span', '', spec.priority)
  );
  byId('spec-actions').replaceChildren();
  spec.actions.forEach(action => {
    const button = element('button', 'primary', action.label);
    button.type = 'button';
    button.setAttribute('aria-label', action.label + ' ' + code);
    button.addEventListener('click', () => previewAction(code, action));
    byId('spec-actions').append(button);
  });
  if (!spec.actions.length) byId('spec-actions').append(element('p', '', 'Nessuna azione disponibile per questo stato.'));
  showDocument('story');
  showView('spec');
}
function openConversation(code = null) {
  conversationSpec = code;
  selectedActivity = null;
  conversationOpen = true;
  const spec = code ? specs.get(code) : null;
  byId('chat-title').textContent = spec ? 'Conversazione · ' + code : 'Nuova conversazione';
  byId('current-thread').textContent = spec ? code + ' · ' + spec.title : 'Nuova conversazione';
  byId('current-thread').hidden = false;
  byId('index-empty').hidden = true;
  byId('chat-spec-link').hidden = !spec;
  byId('chat-spec-link').textContent = spec ? 'Apri ' + code : '';
  byId('action-selection').hidden = !spec;
  byId('selection-text').textContent = spec ? code + ' · ' + spec.title : '';
  byId('message-context').textContent = spec ? 'Spec: ' + code : 'Workspace: ARchetipo';
  byId('message-label').textContent = 'Messaggio';
  byId('message').placeholder = 'Scrivi all’agente…';
  byId('chat-content').replaceChildren();
  if (spec) {
    const empty = element('div', 'empty-chat');
    empty.append(element('p', '', 'Nessun messaggio.'));
    byId('chat-content').append(empty);
  } else {
    renderActivityChooser();
  }
  showView('conversation');
}
document.querySelectorAll('[data-view]').forEach(button => {
  button.addEventListener('click', () => showView(button.dataset.view));
});
byId('back-board').addEventListener('click', () => showView('board'));
byId('discuss-spec').addEventListener('click', () => openConversation(selectedSpec));
byId('chat-spec-link').addEventListener('click', () => openSpec(conversationSpec));
byId('new-conversation').addEventListener('click', () => openConversation());
byId('current-thread').addEventListener('click', () => showView('conversation'));
byId('clear-selection').addEventListener('click', () => openConversation());
document.querySelectorAll('[data-document]').forEach(button => {
  button.addEventListener('click', () => showDocument(button.dataset.document));
});
byId('message-form').addEventListener('submit', event => {
  event.preventDefault();
  if (!byId('message').value.trim()) return;
  if (!conversationOpen) openConversation();
  notice('Mockup: il messaggio non è stato inviato. Il testo resta nel campo.');
});
byId('workspace-button').addEventListener('click', () => notice('Workspace aperto: ARchetipo. Il selettore non è collegato nel mockup.'));
document.querySelectorAll('[data-notice]').forEach(button => {
  button.addEventListener('click', () => notice(button.dataset.notice + ': voce esistente, pannello non riprodotto nel mockup.'));
});
const epics = new Map(data.specs.map(spec => [spec.epic.code, spec.epic.title]));
for (const [code, title] of epics) {
  const option = element('option', '', code + ' · ' + title);
  option.value = code;
  byId('spec-epic').append(option);
}
byId('new-spec').addEventListener('click', () => byId('new-spec-dialog').showModal());
byId('close-spec-dialog').addEventListener('click', () => byId('new-spec-dialog').close());
byId('cancel-spec').addEventListener('click', () => byId('new-spec-dialog').close());
byId('spec-form').addEventListener('submit', event => {
  event.preventDefault();
  notice('Mockup: nessuna spec creata. I campi non sono stati salvati.');
});
renderBoard();
const initialView = new URLSearchParams(location.search).get('view');
showView(initialView === 'board' ? 'board' : 'conversation');

// Le attività preparano il contesto, senza aggiungere comandi al messaggio.
function renderActivityChooser() {
  const hub = element('div', 'activity-hub');
  hub.append(element('p', 'hub-description', 'Scegli un’attività oppure scrivi un messaggio.'));
  const grid = element('div', 'activity-grid');
  for (const [id, entry] of Object.entries(activityEntries)) {
    const button = element('button', 'activity-card');
    button.type = 'button';
    button.dataset.activity = id;
    button.append(element('span', 'activity-title', entry.title));
    button.append(element('span', 'activity-description', entry.description));
    button.append(element('span', 'activity-output', entry.output));
    grid.append(button);
  }
  hub.append(grid);
  const boardLink = element('button', 'text-button', 'Apri la board');
  boardLink.type = 'button';
  boardLink.addEventListener('click', () => showView('board'));
  hub.append(boardLink);
  byId('chat-content').replaceChildren(hub);
}
function selectActivity(id) {
  const entry = activityEntries[id];
  if (!entry) return;
  if (!conversationOpen) openConversation();
  selectedActivity = id;
  byId('chat-title').textContent = entry.title + (conversationSpec ? ' · ' + conversationSpec : '');
  byId('current-thread').textContent = entry.title + (conversationSpec ? ' · ' + conversationSpec : '');
  byId('action-selection').hidden = !conversationSpec;
  byId('message-label').textContent = entry.label;
  byId('message').placeholder = entry.placeholder;
  byId('message-context').textContent = conversationSpec ? 'Spec: ' + conversationSpec : 'Workspace: ARchetipo';
  const context = element('div', 'activity-context');
  context.append(element('p', '', entry.description));
  if (id !== 'free') {
    const result = element('div', 'activity-result');
    result.append(element('span', '', 'Risultato'), element('strong', '', entry.output));
    context.append(result);
  }
  const change = element('button', 'text-button', 'Cambia attività');
  change.type = 'button';
  change.addEventListener('click', () => openConversation());
  context.append(change);
  byId('chat-content').replaceChildren(context);
  byId('activity-menu').open = false;
  byId('message').focus();
}
document.addEventListener('click', event => {
  const button = event.target.closest('[data-activity]');
  if (button) selectActivity(button.dataset.activity);
});
renderActivityChooser();
