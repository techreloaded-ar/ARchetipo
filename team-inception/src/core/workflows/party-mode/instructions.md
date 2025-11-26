# Party Mode - Multi-Agent PRD Discovery Instructions

<critical>The workflow execution engine is governed by: {project_root}/{air_folder}/core/tasks/workflow.xml</critical>
<critical>This workflow orchestrates group discussions between all installed AIRchetipo agents to discover product requirements and automatically generate a comprehensive PRD</critical>
<critical>Communicate all responses in {communication_language} and adapt deeply to user skill level</critical>
<critical>Generate all documents in {document_output_language}</critical>

<workflow>

<step n="0" goal="Load PRD Schema and Reference Materials">
  <action>Load the PRD checklist from {{prd_checklist}}</action>
  <action>Load project types reference from {{project_types}}</action>
  <action>Load domain complexity reference from {{domain_complexity}}</action>
  <action>Initialize internal tracking of PRD completeness</action>
  <note>These references guide the conversation to ensure all necessary information is collected</note>
</step>

<step n="1" goal="Load Agent Manifest and Build Agent Roster">
  <action>Load the agent manifest CSV from {{agent_manifest}}</action>
  <action>Parse CSV to extract all agent entries with their condensed information:</action>
    - name (agent identifier)
    - displayName (agent's persona name)
    - title (formal position)
    - icon (visual identifier)
    - role (capabilities summary)
    - identity (background/expertise)
    - communicationStyle (how they communicate)
    - principles (decision-making philosophy)
    - module (source module)
    - path (file location)

<action>Build complete agent roster with merged personalities</action>
<action>Store agent data for use in conversation orchestration</action>
<note>Special focus on Product Manager, Strategist, UX Designer, and Architect agents for PRD creation</note>
</step>

<step n="2" goal="Initialize Party Mode for PRD Discovery">
  <action>Announce party mode activation with PRD goal</action>
  <action>List all participating agents with their merged information</action>
  <action>Present the PRD information structure that will be collected</action>

  <format>
    🎉 PARTY MODE ATTIVATO - CREAZIONE PRD! 🎉

    Gli agenti AIRchetipo collaboreranno per raccogliere tutte le informazioni necessarie
    a creare un Product Requirements Document completo.

    **Agenti Partecipanti:**
    [For each agent in roster:]
    - [Icon] [Agent Name] ([Title]): [Role from merged data]

    [Total count] agenti pronti a collaborare!

    **Struttura PRD che creeremo insieme:**

    1. 🎯 Vision & Strategic Objectives
    2. 💼 Business Model
    3. 👥 Target Users (Personas & Customer Journeys)
    4. ✅ Success Criteria
    5. 📦 Product Scope (MVP, Growth, Vision)
    6. 🏗️ Project Classification
    7. ⚙️ Functional Requirements
    8. 🚀 Non-Functional Requirements
    9. 🏛️ High-Level Architecture
    10. 📋 Epic Breakdown
    11. 🗺️ Roadmap

    Gli agenti ti faranno domande per raccogliere tutte queste informazioni.
    Quando avremo tutto il necessario, genereremo automaticamente il tuo PRD completo!

    **Iniziamo! Parlaci del prodotto che vuoi creare...**

  </format>

<action>Wait for user to provide initial product description</action>
</step>

<step n="3" goal="Orchestrate PRD Discovery Discussion" repeat="until-complete">
  <action>For each conversation round, agents work to fill the PRD checklist</action>

  <substep n="3a" goal="Determine Discussion Phase and Relevant Agents">
    <action>Check PRD completeness tracker to determine current phase:</action>
      - Phase 1: Discovery (Vision, Business Model, Personas, Journey)
      - Phase 2: Requirements (Scope, FRs, NFRs, Project Classification)
      - Phase 3: Planning (Architecture, Epics, Roadmap)
      - Phase 4: Validation (Review completeness, fill gaps)

    <action>Select 2-3 most relevant agents for current phase:</action>
      - Phase 1: Product Manager, Strategist, UX Designer
      - Phase 2: Product Manager, Architect, Analyst
      - Phase 3: Architect, Product Manager, Analyst
      - Phase 4: All agents for final review

    <action>Identify what information is still missing from checklist</action>
    <note>If user addresses specific agent by name, prioritize that agent</note>

  </substep>

  <substep n="3b" goal="Generate In-Character Discovery Responses">
    <action>For each selected agent, generate authentic response focused on PRD discovery:</action>

    <action>Use the agent's merged personality data:</action>
      - Apply their communicationStyle exactly
      - Reflect their principles in reasoning
      - Draw from their identity and role for expertise
      - Focus questions on missing PRD information

    <action>Agents should:</action>
      - Ask targeted questions to fill PRD checklist gaps
      - Build on previous answers to go deeper
      - Reference and validate information already collected
      - Challenge assumptions constructively
      - Offer insights based on their expertise
      - Connect different aspects of the product vision

    <action>Enable natural cross-talk between agents:</action>
      - Agents can reference each other by name
      - Agents can build on each other's questions
      - Agents can respectfully disagree or offer alternatives
      - Agents can ask follow-up questions to each other
      - Agents can suggest pivoting to different topics if needed

    <important>Agents must LISTEN and BUILD on user's answers, not repeat questions about already-covered topics</important>
    <important>Agents should ACKNOWLEDGE what has been collected and MOVE FORWARD to missing information</important>

  </substep>

  <substep n="3c" goal="Handle Questions and Track Information">
    <check if="an agent asks the user a direct question">
      <action>Clearly highlight the question</action>
      <action>End that round of responses</action>
      <action>WAIT for user input before continuing</action>
      <action>When user responds, EXTRACT and STORE relevant information for PRD</action>
      <action>UPDATE PRD completeness tracker</action>
    </check>

    <check if="agents ask each other questions">
      <action>Allow natural back-and-forth in the same response round</action>
      <action>Maintain conversational flow</action>
    </check>

    <check if="user provides information without being asked">
      <action>EXTRACT and STORE all relevant information for PRD</action>
      <action>Agents acknowledge and build on the information</action>
      <action>UPDATE PRD completeness tracker</action>
    </check>

  </substep>

  <substep n="3d" goal="Format and Present Responses">
    <action>Present each agent's contribution clearly:</action>
    <format>
      **[Icon] [Agent Name]:** [Their response in their voice/style]

      **[Icon] [Another Agent]:** [Their response, potentially referencing the first]

      **[Icon] [Third Agent if selected]:** [Their contribution]
    </format>

    <action>Maintain spacing between agents for readability</action>
    <action>Preserve each agent's unique voice throughout</action>
    <action>Use communication_language for all agent responses</action>

  </substep>

  <substep n="3e" goal="Check PRD Completeness">
    <action>After each user response, update internal PRD completeness tracker</action>

    <check if="minimum required information collected">
      <required-minimum>
        - Vision statement
        - At least 1 complete persona
        - Product scope (at least MVP defined)
        - Project classification (type, domain, complexity)
        - At least 10 functional requirements
        - High-level architecture basics
      </required-minimum>
    </check>

    <check if="all recommended information collected">
      <recommended>
        - 2 personas with customer journeys
        - Business model
        - Success criteria
        - Project-specific requirements
        - Relevant non-functional requirements
        - Epic breakdown
        - Roadmap phases
      </recommended>
    </check>

    <action>Every 3-4 conversation rounds, provide a progress update:</action>
    <format>
      ---
      **📊 Progresso PRD:**
      ✅ Completato: [List completed sections]
      🔄 In corso: [Current section being discussed]
      ⏳ Mancante: [List missing critical sections]
      ---
    </format>

    <check if="minimum required information complete">
      <action>Proceed to Step 4 (PRD Generation)</action>
    </check>

    <check if="missing critical information">
      <action>Continue discussion, focusing on gaps</action>
    </check>

    <check if="discussion becomes circular or stuck">
      <action>Virgilio (AIRchetipo Master) or Product Manager summarizes progress</action>
      <action>Redirect to specific missing information</action>
      <action>If truly stuck, offer to proceed with available information</action>
    </check>

  </substep>

  <substep n="3f" goal="Check for Manual Exit">
    <check if="user message contains any {{exit_triggers}}">
      <action>Ask if user wants to:</action>
        A) Generate PRD with current information
        B) Exit without generating PRD
      <action>Proceed based on user choice</action>
    </check>
  </substep>
</step>

<step n="4" goal="Generate PRD Automatically">
  <action>Announce PRD generation with enthusiasm</action>

  <format>
    🎊 **INFORMAZIONI COMPLETE!** 🎊

    Abbiamo raccolto tutte le informazioni necessarie per creare il tuo PRD!

    Gli agenti stanno ora sintetizzando tutto in un documento strutturato...

  </format>

<action>Synthesize all collected information from the conversation</action>
<action>Organize information according to PRD template structure</action>

<action>Generate comprehensive PRD content for each section:</action>

  <section name="Executive Summary">
    <action>Synthesize vision alignment from collected information</action>
    <action>Articulate product differentiator clearly</action>
    <template-output>vision_alignment</template-output>
    <template-output>product_differentiator</template-output>
  </section>

  <section name="Vision">
    <action>Format vision statement</action>
    <action>List strategic objectives (3-5)</action>
    <action>Describe long-term impact</action>
    <template-output>vision_statement</template-output>
    <template-output>strategic_objectives</template-output>
    <template-output>long_term_impact</template-output>
  </section>

  <section name="Business Model">
    <action>Structure business model canvas with 5 key areas</action>
    <template-output>business_model_canvas</template-output>
  </section>

  <section name="Target Users">
    <action>Format Persona 1 with complete profile</action>
    <action>Format Persona 2 with complete profile (if collected)</action>
    <template-output>persona_1_name</template-output>
    <template-output>persona_1_profile</template-output>
    <template-output>persona_2_name</template-output>
    <template-output>persona_2_profile</template-output>
  </section>

  <section name="Customer Journey">
    <action>Map journey stages for each persona</action>
    <template-output>persona_1_journey</template-output>
    <template-output>persona_2_journey</template-output>
  </section>

  <section name="Project Classification">
    <action>Document project type, domain, complexity</action>
    <template-output>project_type</template-output>
    <template-output>domain_type</template-output>
    <template-output>complexity_level</template-output>
    <template-output>project_classification</template-output>
    <check if="complex domain">
      <template-output>domain_context_summary</template-output>
    </check>
  </section>

  <section name="Success Criteria">
    <action>Format success criteria with specific metrics</action>
    <template-output>success_criteria</template-output>
    <check if="business metrics collected">
      <template-output>business_metrics</template-output>
    </check>
  </section>

  <section name="Product Scope">
    <action>Organize features by MVP, Growth, Vision</action>
    <template-output>mvp_scope</template-output>
    <template-output>growth_features</template-output>
    <template-output>vision_features</template-output>
  </section>

  <section name="Domain-Specific Requirements" optional="true">
    <check if="domain requirements collected">
      <template-output>domain_considerations</template-output>
    </check>
  </section>

  <section name="Innovation & Novel Patterns" optional="true">
    <check if="innovation patterns identified">
      <template-output>innovation_patterns</template-output>
      <template-output>validation_approach</template-output>
    </check>
  </section>

  <section name="Project-Specific Requirements">
    <action>Format requirements based on project type</action>
    <template-output>project_type_requirements</template-output>
    <check if="API/Backend">
      <template-output>endpoint_specification</template-output>
      <template-output>authentication_model</template-output>
    </check>
    <check if="Mobile">
      <template-output>platform_requirements</template-output>
      <template-output>device_features</template-output>
    </check>
    <check if="SaaS B2B">
      <template-output>tenant_model</template-output>
      <template-output>permission_matrix</template-output>
    </check>
  </section>

  <section name="UX Principles" optional="true">
    <check if="UX information collected">
      <template-output>ux_principles</template-output>
      <template-output>key_interactions</template-output>
    </check>
  </section>

  <section name="Functional Requirements">
    <critical>This is THE CAPABILITY CONTRACT for all downstream work</critical>
    <action>Organize FRs by capability area</action>
    <action>Number sequentially (FR1, FR2, FR3...)</action>
    <action>Ensure completeness - every capability discussed must have an FR</action>
    <template-output>functional_requirements_complete</template-output>
  </section>

  <section name="Non-Functional Requirements">
    <action>Include only relevant NFR categories</action>
    <check if="performance matters">
      <template-output>performance_requirements</template-output>
    </check>
    <check if="security matters">
      <template-output>security_requirements</template-output>
    </check>
    <check if="scalability matters">
      <template-output>scalability_requirements</template-output>
    </check>
    <check if="accessibility matters">
      <template-output>accessibility_requirements</template-output>
    </check>
    <check if="integration matters">
      <template-output>integration_requirements</template-output>
    </check>
  </section>

  <section name="High-Level Architecture">
    <action>Document architecture overview and technology choices</action>
    <template-output>high_level_architecture</template-output>
    <template-output>technology_stack</template-output>
    <template-output>database_architecture</template-output>
    <template-output>frameworks_and_libraries</template-output>
    <check if="infrastructure discussed">
      <template-output>infrastructure_overview</template-output>
    </check>
  </section>

  <section name="Epic Breakdown">
    <action>Transform FRs into implementable epics with stories</action>
    <action>Organize by priority (MVP/Growth/Vision)</action>
    <template-output>epics_overview</template-output>
    <template-output>epic_details</template-output>
  </section>

  <section name="Roadmap">
    <action>Sequence epics into phases/quarters</action>
    <template-output>roadmap_phases</template-output>
  </section>

  <section name="References">
    <check if="input documents used">
      <template-output>product_brief_path</template-output>
      <template-output>domain_brief_path</template-output>
      <template-output>research_documents</template-output>
    </check>
  </section>

  <section name="Final Summary">
    <template-output>product_value_summary</template-output>
  </section>

<action>Apply PRD template: {{prd_template}}</action>
<action>Write complete PRD to: {{default_output_file}}</action>
<action>Ensure all template variables are populated</action>
<action>Use {document_output_language} for all document content</action>
</step>

<step n="5" goal="Present PRD and Exit Party Mode">
  <action>Have 2-3 agents celebrate the completion with characteristic responses</action>
  <action>Presenta un menù post-PRD con i possibili passi successivi e chiedi all'utente di scegliere prima di concludere</action>
  <action>Opzione 1: proporre la creazione di un backlog di user stories complete di acceptance criteria tramite `workflow create-epics-and-stories`</action>
  <action>Opzione 2: proporre la generazione di un documento di architettura tecnica completo tramite `workflow architecture`</action>
  <action>Descrivi l'output atteso per ogni opzione e attendi esplicitamente che l'utente risponda con la preferenza desiderata</action>

  <format>
    ✅ **PRD COMPLETO!** ✅

    **[Icon] [Product Manager]:** [Enthusiastic acknowledgment of completed PRD]

    **[Icon] [Architect]:** [Technical validation comment]

    **[Icon] [Strategist]:** [Strategic perspective on next steps]

    ---

    📄 **Il tuo PRD è stato creato con successo!**

    **File generato:** {{default_output_file}}

    **Contenuto del PRD:**
    - ✅ Vision & Strategic Objectives
    - ✅ Business Model
    - ✅ User Personas & Customer Journeys
    - ✅ Success Criteria
    - ✅ Product Scope (MVP, Growth, Vision)
    - ✅ Project Classification
    - ✅ Functional Requirements
    - ✅ Non-Functional Requirements
    - ✅ High-Level Architecture
    - ✅ Epic Breakdown with User Stories
    - ✅ Roadmap

    **Menù post-PRD (seleziona un'opzione):**

    1. 📋 **Crea Backlog con User Stories + Acceptance Criteria**
       - Output: documento dettagliato con epics, user stories e acceptance criteria pronti per la pianificazione
       - Esegui: `workflow create-epics-and-stories`

    2. 🏛️ **Crea Documento di Architettura Tecnica Completo**
       - Output: architettura collaborativa con decisioni tecniche, stack e linee guida di implementazione
       - Esegui: `workflow architecture`

    Rispondi indicandomi 1 o 2 (puoi anche chiedere altro) e ti guiderò immediatamente nel percorso scelto.

    ---

    🎊 **Grazie per questa fantastica sessione di discovery collaborativa!**

    Il tuo prodotto - {product_value_summary} - ha ora una solida base documentale.

  </format>

  <action>Exit workflow</action>
</step>


</workflow>

## Role-Playing Guidelines for PRD Discovery

<guidelines>
  <guideline>Keep all responses strictly in-character based on merged personality data</guideline>
  <guideline>Use each agent's documented communication style consistently</guideline>
  <guideline>Focus questions on PRD information gaps, not generic conversation</guideline>
  <guideline>Listen actively to user responses and avoid repeating questions</guideline>
  <guideline>Acknowledge collected information and build on it progressively</guideline>
  <guideline>Allow natural disagreements and different perspectives on product direction</guideline>
  <guideline>Maintain professional discourse while being engaging and enthusiastic</guideline>
  <guideline>Let agents reference each other naturally by name or role</guideline>
  <guideline>Connect different aspects of product vision across conversation</guideline>
  <guideline>Challenge assumptions constructively to deepen understanding</guideline>
  <guideline>Respect each agent's expertise boundaries</guideline>
  <guideline>Use {communication_language} for all agent communication with user</guideline>
</guidelines>

## Question Strategy Protocol

<question-strategy>
  <phase-based-questions>
    Phase 1 (Discovery): Open-ended, exploratory, "why" questions
    Phase 2 (Requirements): Specific, technical, "what" and "how" questions
    Phase 3 (Planning): Feasibility, sequencing, "when" and "how much" questions
    Phase 4 (Validation): Clarification, edge cases, completeness checks
  </phase-based-questions>

  <direct-to-user>
    When agent asks user a specific question:
    - End that round immediately after the question
    - Clearly highlight the questioning agent and their question
    - Wait for user response before any agent continues
    - Extract and store all relevant PRD information from response
  </direct-to-user>

  <avoid-repetition>
    Before asking a question, agents must:
    - Check if information was already provided
    - Acknowledge what's been collected
    - Move forward to new information needs
    - Build on previous answers, don't restart topics
  </avoid-repetition>

  <inter-agent>
    Agents can question each other and respond naturally within same round
    This adds depth and shows different perspectives
  </inter-agent>
</question-strategy>

## Information Extraction Protocol

<extraction-protocol>
  <continuous-extraction>
    After EVERY user response:
    1. Scan for all PRD-relevant information
    2. Extract and categorize by PRD section
    3. Store in internal PRD tracker
    4. Update completeness status
    5. Identify remaining gaps
  </continuous-extraction>

  <implicit-information>
    Extract information even when not directly stated:
    - Infer project type from description
    - Detect domain complexity from context
    - Identify implicit requirements from goals
    - Recognize unstated assumptions
    - Then VALIDATE inferences with user
  </implicit-information>

  <building-context>
    Connect information across conversation:
    - Link personas to functional requirements
    - Connect vision to success criteria
    - Relate scope to roadmap phases
    - Ensure consistency across sections
  </building-context>
</extraction-protocol>

## Progress Tracking Protocol

<progress-tracking>
  <internal-tracker>
    Maintain internal state of PRD completeness:
    - Mark collected sections as ✅ Complete
    - Mark partially filled sections as 🔄 In Progress
    - Mark empty sections as ⏳ Pending
    - Prioritize critical missing information
  </internal-tracker>

  <periodic-updates>
    Every 3-4 conversation rounds, show progress:
    - What's been completed
    - What's currently being discussed
    - What's still missing
    - Estimated remaining questions
  </periodic-updates>

  <adaptive-pacing>
    Adjust conversation pace based on:
    - User's detail level and engagement
    - Complexity of the product
    - Time constraints (if mentioned)
    - Information quality (may need deeper exploration)
  </adaptive-pacing>
</progress-tracking>

## PRD Generation Quality Standards

<quality-standards>
  <completeness>
    Minimum required for PRD generation:
    - Vision statement (1 sentence minimum)
    - At least 1 complete persona
    - MVP scope defined
    - Project classification (type, domain, complexity)
    - At least 10 functional requirements
    - Basic architecture direction
  </completeness>

  <consistency>
    Ensure throughout PRD:
    - Personas align with business model
    - FRs support vision and scope
    - Architecture supports FRs and NFRs
    - Epics cover all FRs
    - Roadmap sequences epics logically
  </consistency>

  <clarity>
    All PRD content must be:
    - Clear and unambiguous
    - Actionable for downstream work
    - Appropriate altitude (strategic vs tactical)
    - Professional and well-structured
  </clarity>

  <language>
    All PRD document content in {document_output_language}
    All agent conversation in {communication_language}
  </language>
</quality-standards>

## Moderation and Edge Cases

<moderation>
  <stuck-discussions>
    If conversation becomes circular or stuck:
    1. Virgilio or Product Manager summarizes what's collected
    2. Explicitly lists what's still needed
    3. Offers options: continue, skip optional items, or generate with current info
  </stuck-discussions>

  <insufficient-information>
    If user can't provide certain information:
    1. Acknowledge it's okay
    2. Explain why it's needed (if critical)
    3. Offer to make reasonable assumptions (if optional)
    4. Document gaps in PRD with TODOs
  </insufficient-information>

  <scope-creep>
    If product description keeps expanding:
    1. Product Manager gently guides back to MVP focus
    2. Capture expansion ideas in Growth/Vision sections
    3. Help prioritize ruthlessly
  </scope-creep>

  <technical-depth>
    Adapt technical depth to user skill level:
    - Beginner: More explanations, simpler terms
    - Intermediate: Standard technical language
    - Expert: Deep technical discussions, advanced concepts
  </technical-depth>
</moderation>
