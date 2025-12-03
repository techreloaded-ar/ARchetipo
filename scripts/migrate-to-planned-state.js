#!/usr/bin/env node

/**
 * Migrazione Automatica: TODO → PLANNED
 *
 * Questo script scansiona tutte le storie esistenti nel backlog,
 * trova quelle con sezione Tasks già generata, e aggiorna:
 * - Status: TODO → PLANNED nei file story
 * - Checkbox: [ ] → [P] in backlog.md
 */

const fs = require('fs');
const path = require('path');

const BACKLOG_PATH = 'docs/backlog.md';
const STORIES_DIR = 'docs/stories';

// Regex patterns
const STORY_LINK_PATTERN = /- \[ \] \[US-(\d{3})\]\(stories\/(US-\d{3}-[^)]+\.md)\)/g;
const STATUS_PATTERN = /\*\*Status:\*\* TODO/;
const TASKS_SECTION_PATTERN = /^## Tasks\s*$/m;
const TASK_ITEM_PATTERN = /^- \[ \] TK-\d{3}:/m;

function main() {
  console.log('🔍 Scanning stories in backlog...\n');

  // 1. Leggi backlog.md
  if (!fs.existsSync(BACKLOG_PATH)) {
    console.error(`❌ File ${BACKLOG_PATH} non trovato.`);
    process.exit(1);
  }

  let backlogContent = fs.readFileSync(BACKLOG_PATH, 'utf-8');

  // 2. Estrai tutti i link story TODO
  const todoStories = [];
  let match;
  const regex = new RegExp(STORY_LINK_PATTERN);

  while ((match = regex.exec(backlogContent)) !== null) {
    todoStories.push({
      id: match[1],
      filename: match[2],
      fullMatch: match[0]
    });
  }

  if (todoStories.length === 0) {
    console.log('ℹ️  Nessuna storia TODO trovata nel backlog.');
    return;
  }

  console.log(`Trovate ${todoStories.length} storie TODO.\n`);

  let migratedCount = 0;
  let skippedCount = 0;
  const migratedStories = [];

  // 3. Per ogni storia TODO
  for (const story of todoStories) {
    const storyPath = path.join(STORIES_DIR, story.filename);

    if (!fs.existsSync(storyPath)) {
      console.log(`⚠️  US-${story.id}: File non trovato (${storyPath})`);
      skippedCount++;
      continue;
    }

    let storyContent = fs.readFileSync(storyPath, 'utf-8');

    // 4. Controlla se ha Tasks section con almeno un task
    const hasTasksSection = TASKS_SECTION_PATTERN.test(storyContent);
    const hasTaskItems = TASK_ITEM_PATTERN.test(storyContent);

    if (hasTasksSection && hasTaskItems) {
      // Conta i task
      const taskMatches = storyContent.match(/^- \[ \] TK-\d{3}:/gm);
      const taskCount = taskMatches ? taskMatches.length : 0;

      // 5a. Aggiorna Status: TODO → PLANNED nel file story
      const updatedStoryContent = storyContent.replace(
        STATUS_PATTERN,
        '**Status:** PLANNED'
      );

      if (updatedStoryContent === storyContent) {
        console.log(`⏭️  US-${story.id}: Già PLANNED o Status non trovato`);
        skippedCount++;
        continue;
      }

      fs.writeFileSync(storyPath, updatedStoryContent, 'utf-8');

      // 5b. Aggiorna checkbox: [ ] → [P] in backlog.md
      const oldCheckbox = `- [ ] [US-${story.id}]`;
      const newCheckbox = `- [P] [US-${story.id}]`;
      backlogContent = backlogContent.replace(oldCheckbox, newCheckbox);

      console.log(`✅ US-${story.id} → PLANNED (${taskCount} tasks)`);
      migratedCount++;
      migratedStories.push(`US-${story.id}`);
    } else {
      console.log(`⏭️  US-${story.id} remains TODO (no tasks)`);
      skippedCount++;
    }
  }

  // 6. Scrivi backlog.md aggiornato
  if (migratedCount > 0) {
    fs.writeFileSync(BACKLOG_PATH, backlogContent, 'utf-8');
  }

  // 7. Report finale
  console.log('\n' + '='.repeat(60));
  console.log('📊 Migration complete:\n');
  console.log(`   - Migrated to PLANNED: ${migratedCount} stories`);
  console.log(`   - Remain TODO: ${skippedCount} stories`);
  console.log(`   - Total: ${todoStories.length} stories\n`);

  if (migratedCount > 0) {
    console.log('💾 Updated files:');
    console.log(`   - ${BACKLOG_PATH}`);
    migratedStories.forEach(id => {
      const story = todoStories.find(s => `US-${s.id}` === id);
      if (story) {
        console.log(`   - ${STORIES_DIR}/${story.filename}`);
      }
    });
  }

  console.log('\n✅ Migrazione completata con successo!');
}

// Esegui
try {
  main();
} catch (error) {
  console.error('\n❌ Errore durante la migrazione:', error.message);
  process.exit(1);
}
