# FREE UX Guidelines — Mockup self-check checklist (run before delivering any screen)

Source: consolidated from all chapters of "FREE UX Guidelines" v1.3.
Applies to: every HTML/CSS mockup produced by the FREE Mockup Designer agent.
Keywords: checklist, quality gate, review, verify, compliance, controllo, verifica, conformità, consegna, definition of done.

## How to use this checklist

Context: FREE UX Guidelines > Mockup checklist > How to use. Use when: you have a draft HTML mockup and must validate it before answering.
Go through every group. Each item is pass/fail. Fix every fail, then report the result as a compact list ("Shell 6/6, Layout 5/5, ...") at the end of the answer. If an item does not apply (e.g. no table on the page) mark it "n/a". A mockup with any fail must not be delivered.

## A. Page shell (file 01)

Context: FREE UX Guidelines > Mockup checklist > Shell. Use when: the screen is a full page.
1. Header present: full width, teal `--free-teal-primary`, 64 px, logo left, user area/avatar right.
2. Main menu present: 72 px teal left rail, every item has icon AND label, current section marked as active (white pill, teal icon).
3. Quick Actions: either absent or a mint `--free-mint` vertical tab on the right edge (56 px) with icon-only circular actions with aria-label.
4. Page content on `--free-page-bg`, page header with title (24 px bold), optional subtitle, breadcrumbs when the page is deeper than level 1, back link on second-level/detail pages.
5. Page-level actions in the page header on the right, primary on the far right.
6. Nothing else outside the shell (no floating brand elements, no external footers).

## B. Grid, spacing, responsiveness (file 02)

Context: FREE UX Guidelines > Mockup checklist > Grid. Use when: placing widgets, cards, forms.
1. Content uses the 12-column grid with 24 px gutter; widgets span 6 or 12 columns.
2. All paddings, gaps and margins are multiples of 4 px (4/8/12/16/24/32/48).
3. Widgets and cards: white surface, 16 px radius, light card shadow, 24 px inner padding, 24 px gap between them.
4. The page renders correctly at 1280 px wide and degrades at 1024 px (no horizontal scroll, widgets stack to 12 columns below 1024 px).
5. No fixed pixel widths on content containers except those stated by FREE (menu 72, quick actions 56, min button width 150, text area 328).

## C. Navigation (files 03, 04, 09)

Context: FREE UX Guidelines > Mockup checklist > Navigation. Use when: the screen has tabs, breadcrumbs or leads to other pages.
1. Context tabs (tab di contesto) used only to switch context inside the same page tree; page tabs (tab in pagina) used only to split content of one page; the two are never mixed in the same bar.
2. Active tab: teal label with 3 px teal underline; inactive: secondary text; disabled: `--free-disabled`.
3. Breadcrumbs show the path from the section root, current page not clickable, separators "/".
4. Second-level pages open in place with a back link; complex content is not forced into a modal.

## D. Actions (file 05)

Context: FREE UX Guidelines > Mockup checklist > Actions. Use when: the screen has any button or link.
1. Exactly one primary CTA per view (mint background, dark teal uppercase label, min width 150 px, pill radius).
2. CTA order: primary on the right, secondary/tertiary on the left; three CTAs = two on the right, least important on the left; vertical stacks ordered top→bottom by importance.
3. In forms the CTA row is 32 px below the last field and not sticky.
4. Disabled and loading states styled with `--free-disabled` / spinner; no custom colors. Urgent variant (magenta `--free-urgent`, white label, no icon) used only for time-critical actions.
5. Icon-only buttons (circle, icon button) carry aria-label; text links are uppercase teal and used only for low-weight actions.

## E. Forms (file 06)

Context: FREE UX Guidelines > Mockup checklist > Forms. Use when: the screen contains inputs.
1. Max 3 fields per row (4 columns each); similar fields have equal widths; related fields grouped under a section title.
2. Every field has a floating label on the top border; optional fields say "(Opzionale)"; helper text ≤ 2 lines, left aligned under the field.
3. Error fields: border and message in error color, message under the field replacing the helper text; a summary banner if the form has several errors.
4. Icons inside inputs are decorative or clear/search; no clickable actions inside the field box.
5. Correct control for the data type: select for ≤ 1 choice from a list, multiselect for many, radio for 2-5 exclusive visible options, checkbox for independent options, toggle for immediate on/off, date/time pickers for dates and times.

## F. Tables, lists, paginator (files 07, 08)

Context: FREE UX Guidelines > Mockup checklist > Tables and lists. Use when: the screen lists records.
1. Table inside a white rounded container with a toolbar (search/filters left, actions right).
2. Header row: dark teal uppercase 12 px labels, sort icon on sortable columns, 2 px teal bottom border.
3. Alignment: ALL cell content left aligned (numbers and amounts too, with tabular figures), status as colored dot + label, max 3 inline row actions in the last column, more actions in a contextual menu (max 5 items).
4. Row hover and selection use `--free-teal-tint-100`; row height consistent (min 56 px).
5. Numerical paginator bottom right (max 7 page buttons, "..." when more, first/last and prev/next always visible); simple dot paginator only for carousels/cards (max 5 dots).
6. Empty state and loading (skeleton) provided for the table/list.

## G. Overlays: modals, tooltips, dropdowns (file 10)

Context: FREE UX Guidelines > Mockup checklist > Overlays. Use when: the screen shows a modal, tooltip or menu.
1. Modal backdrop black 75 %; modal white, 16 px radius, title centred and dark, X close top-right, divider above the footer, primary CTA right / secondary left (Small: stacked and centred, primary first), max 2 CTAs, single acknowledge CTA is secondary.
2. Modal size is one of Small 300 / Default 600 / Large 800 / Extra large 1140 px, max height 650 px; body scrolls, header and footer do not; no modal inside a modal.
3. Tooltips: short text (max 250 px, 5-6 lines), appear on hover after 300-500 ms, positioned below the trigger and left aligned, no arrow, dark navy background, at most one 18 px icon action.
4. Dropdown/select panels: 8 px radius, overlay shadow, selected/hover option on `--free-teal-tint-100`.

## H. Feedback and states (file 13)

Context: FREE UX Guidelines > Mockup checklist > Feedback. Use when: the flow includes results of an action or data loading.
1. Banners use the four FREE types with the correct color pair (icon/text full color, background 10 %); one banner at a time; 100 % width of the grid; close icon when not auto-dismissed.
2. Success/error after an action is shown by banner or confirmation modal, not by changing page unexpectedly.
3. Every asynchronous area has a loading representation (loader or skeleton) and an error state with retry.
4. Empty states have icon, short title, explanation and, when useful, a CTA.

## I. Progress and search (files 11, 12)

Context: FREE UX Guidelines > Mockup checklist > Progress and search. Use when: the screen is a wizard or has search.
1. Multi-step flows show a step bar with max 5 steps: completed and current steps teal filled with white number (current has bold label), upcoming steps outlined muted; labels right of the dot when < 5 steps, below when 5; irreversible completed steps show a padlock. Longer processes use the timeline component.
2. Wizard navigation: "Indietro" on the left, "Avanti"/"Conferma" primary on the right.
3. Simple search: pill field with search icon, placeholder describing what can be searched, positioned in the toolbar of the list it filters.
4. Advanced search: filters laid out on the grid, "Applica" primary right, "Reimposta" left, applied filters shown as chips, no-results banner anchored under the section.

## J. Charts (file 14)

Context: FREE UX Guidelines > Mockup checklist > Charts. Use when: the screen has data visualisation.
1. Chart type fits the data (comparison → bar, trend → line, part-of-whole → donut with ≤ 6 slices).
2. Series colors follow `--free-chart-1..8` in order; legend present and readable; values labelled or available via tooltip.
3. Charts live inside a widget with title and, if needed, period selector; axes and gridlines in muted teal/grey.

## K. Accessibility (file 15)

Context: FREE UX Guidelines > Mockup checklist > Accessibility. Use when: always.
1. Text contrast ≥ 4.5:1, large text and UI components ≥ 3:1 (do not put `--free-text-secondary` on tinted backgrounds).
2. Keyboard focus visible on every interactive element (`--free-focus-ring`), logical tab order.
3. Semantic HTML: header/nav/main/section, buttons are <button>, tables use <th scope>, form controls have associated labels, images/icons have alt or aria-hidden.
4. Color is never the only carrier of meaning (status has text or icon too).
5. Minimum body text 14 px, helper 12 px, line height ≥ 1.4.

## L. Tokens and code hygiene (files 16, 17)

Context: FREE UX Guidelines > Mockup checklist > Tokens. Use when: always.
1. Only `--free-*` variables and `free-` classes; no raw hex outside the :root block; no external CSS/JS/fonts/icons.
2. Fonts: heading Montserrat stack, body Open Sans stack; button labels uppercase 14 px bold.
3. Realistic Italian banking copy (clienti, NDG, portafogli, pratiche, importi in euro with thousands separator and comma decimals); no lorem ipsum in labels, CTAs or headers.
4. Single self-contained HTML file, valid HTML5, <title> equal to the screen name.
