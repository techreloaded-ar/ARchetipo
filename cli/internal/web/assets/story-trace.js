(function () {
 "use strict";
 const esc = text => String(text ?? "").replace(/[&<>"']/g, char => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[char]));
 function renderWorks(works) {
  if (!Array.isArray(works) || !works.length) return '<p class="docs-note">Nessun lavoro collegato. Puoi iniziare da questa storia.</p>';
  return '<h3>Lavori collegati</h3><ul>' + works.map(work => '<li>' + (work.conversation_id ? '<button type="button" class="ghost-btn" data-linked-conversation="'+esc(work.conversation_id)+'">'+esc(work.title)+'</button>' : '<strong>'+esc(work.title)+'</strong>') + '<span>'+esc(work.status_label)+'</span>' + (/^https?:\/\//.test(work.pr_url || "") ? '<a target="_blank" rel="noopener noreferrer" href="'+esc(work.pr_url)+'">Apri PR ↗</a>' : '') + '</li>').join("") + '</ul>';
 }
 function renderStories(stories) {
  if (!Array.isArray(stories) || !stories.length) return "";
  return '<h3>Storie collegate</h3><ul class="story-links">'+stories.map(story => '<li><button type="button" class="ghost-btn" data-linked-story="'+esc(story.code)+'">'+esc(story.code+' · '+story.title)+'</button><span>'+esc(story.status_label || "")+'</span></li>').join("")+'</ul>';
 }
 if (typeof module !== "undefined" && module.exports) module.exports = {renderWorks, renderStories};
 if (typeof window !== "undefined") window.StoryTrace = {renderWorks, renderStories};
})();
