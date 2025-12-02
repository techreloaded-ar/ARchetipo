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
    🎉 PARTY MODE ACTIVATED - PRD CREATION 🎉

    AIRchetipo agents will collaborate to gather every piece of information required
    to produce a comprehensive Product Requirements Document.

    **Participating Agents:**
    [For each agent in roster:]
    - [Icon] [Agent Name] ([Title]): [Role from merged data]

    [Total count] agents stand ready to contribute!

    **PRD Structure We Will Build Together:**

    1. 🎯 Vision & Strategic Objectives
    2. 💼 Business Model
    3. 👥 Target Users (Personas & Customer Journeys)
    4. ✅ Success Criteria
    5. 📦 Product Scope (MVP, Growth, Vision)
    6. 🏗️ Project Classification
    7. 📐 Technical Architecture
    8. ⚙️ Functional Requirements
    9. 🚀 Non-Functional Requirements
    10. 📋 Epic Breakdown
    11. 🗺️ Roadmap

    Agents will ask focused questions to gather each section above.
    Once we have everything we need, we will automatically generate your complete PRD!

    **Let's begin! Tell us about the product you want to create...**

  </format>

<action>Wait for user to provide initial product description</action>
</step>

<step n="3" goal="Orchestrate PRD Discovery Discussion" repeat="until-complete">
  <action>For each conversation round, agents work to fill the PRD checklist</action>

  <substep n="3a" goal="Determine Discussion Phase and Relevant Agents">
    <action>Check PRD completeness tracker to determine current phase:</action>
      - Phase 1: Discovery (Vision, Business Model, Personas, Journey, Scope, Project Classification)
      - Phase 2: Technical Architecture (MANDATORY - Architect leads)
      - Phase 3: Requirements (FRs, NFRs, Domain-specific)
      - Phase 4: Planning (Epics, Roadmap)
      - Phase 5: Validation (Review completeness, fill gaps)

    <action>Select 2-3 most relevant agents for current phase:</action>
      - Phase 1: Product Manager, Strategist, UX Designer
      - Phase 2: Architect MUST lead, Product Manager provides context
      - Phase 3: Product Manager, Analyst, Architect (for technical feasibility)
      - Phase 4: Architect, Product Manager, Analyst
      - Phase 5: All agents for final review

    <action>Identify what information is still missing from checklist</action>
    <note>If user addresses specific agent by name, prioritize that agent</note>
    <critical>Phase 2 (Technical Architecture) is MANDATORY and cannot be skipped. Architect must provide a complete architectural proposal before proceeding to Phase 3.</critical>

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

    <special-instructions for="Architect" phase="Phase 2: Technical Architecture">
      <critical>When Phase 2 begins, Architect MUST take the lead and propose a complete technical architecture.</critical>

      <action>Architect should:</action>
        1. **Analyze** project type, domain, requirements, and constraints collected in Phase 1
        2. **Propose** a concrete architectural pattern (e.g., "Modular Monolith", "Microservices", "Serverless")
        3. **Specify** exact technologies with versions:
           - Programming language(s) (e.g., "TypeScript 5.3 with Node.js 20.x")
           - Backend framework (e.g., "NestJS 10.x" or "Express 4.18")
           - Frontend framework if needed (e.g., "React 18.2 with Next.js 14")
           - Database (e.g., "PostgreSQL 16 with Prisma ORM 5.x")
        4. **Design** directory structure with clear organization pattern
        5. **Define** development environment setup (Docker, local, cloud-based)
        6. **Outline** build and CI/CD pipeline approach
        7. **Specify** deployment strategy and infrastructure
        8. **Justify** each major decision based on:
           - Project requirements and constraints
           - Scalability and performance needs
           - Team expertise (if known) or industry best practices

      <format>
        Architect should present the architecture as a PROPOSAL, not questions:

        "Based on what we've discussed, I propose the following architecture:

        **System Architecture:** [Pattern with justification]
        **Technology Stack:** [Specific technologies with versions]
        **Database:** [Specific choice with rationale]
        ...

        This approach will [explain benefits for this specific project].

        What do you think? Any constraints or preferences I should consider?"
      </format>

      <note>Architect can ask clarifying questions FIRST if critical information is missing, but MUST then provide a concrete proposal.</note>
      <note>The proposal should be specific enough that a developer could start setting up the project immediately.</note>
    </special-instructions>

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
    🎊 **ALL REQUIRED INFORMATION COLLECTED!** 🎊

    We gathered every detail needed to create your PRD.

    The agents are now synthesizing the conversation into a structured document...

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

  <section name="Technical Architecture">
    <critical>This section is MANDATORY and must be complete. Architect is responsible for all architectural decisions.</critical>

    <action>Document complete technical architecture proposed by the Architect</action>

    <subsection name="System Architecture">
      <template-output>high_level_architecture</template-output>
      <template-output>architecture_pattern</template-output>
      <template-output>architecture_components</template-output>
    </subsection>

    <subsection name="Technology Stack">
      <template-output>technology_stack</template-output>
      <template-output>programming_languages</template-output>
      <template-output>backend_framework</template-output>
      <check if="frontend exists">
        <template-output>frontend_framework</template-output>
      </check>
    </subsection>

    <subsection name="Database and Persistence">
      <template-output>database_architecture</template-output>
      <template-output>database_technology</template-output>
      <template-output>data_modeling_approach</template-output>
      <template-output>migration_strategy</template-output>
    </subsection>

    <subsection name="Frameworks and Libraries">
      <template-output>frameworks_and_libraries</template-output>
      <template-output>auth_libraries</template-output>
      <template-output>api_libraries</template-output>
      <template-output>testing_libraries</template-output>
      <template-output>logging_libraries</template-output>
      <template-output>validation_libraries</template-output>
    </subsection>

    <subsection name="Project Structure">
      <template-output>directory_structure</template-output>
      <template-output>code_organization_pattern</template-output>
      <template-output>directory_layout_example</template-output>
    </subsection>

    <subsection name="Development Environment">
      <template-output>development_environment</template-output>
      <template-output>required_dev_tools</template-output>
      <template-output>local_setup_approach</template-output>
      <template-output>environment_config_management</template-output>
    </subsection>

    <subsection name="Build and CI/CD">
      <template-output>build_pipeline</template-output>
      <template-output>build_tool</template-output>
      <template-output>testing_strategy_pipeline</template-output>
      <template-output>deployment_automation</template-output>
      <template-output>environment_promotion_strategy</template-output>
    </subsection>

    <subsection name="Deployment Strategy">
      <template-output>deployment_strategy</template-output>
      <template-output>target_infrastructure</template-output>
      <template-output>containerization_approach</template-output>
      <template-output>hosting_model</template-output>
      <template-output>scaling_strategy</template-output>
      <template-output>deployment_pattern</template-output>
    </subsection>

    <subsection name="Infrastructure">
      <template-output>infrastructure_overview</template-output>
      <template-output>compute_resources</template-output>
      <template-output>storage_solutions</template-output>
      <template-output>networking_setup</template-output>
      <check if="cdn needed">
        <template-output>cdn_services</template-output>
      </check>
      <template-output>monitoring_tools</template-output>
    </subsection>

    <subsection name="Architecture Decisions">
      <action>Document key architectural decisions and their rationale</action>
      <template-output>architecture_decisions</template-output>
    </subsection>

    <note>All architectural choices must be concrete and specific (e.g., "Node.js 20.x with Express 4.18" not just "Node.js backend")</note>
    <note>Architect must justify technology choices based on project requirements, scalability needs, and team constraints</note>
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

<step n="5" goal="Transform the PRD into a complete product backlog">
  <critical>Continue immediately after PRD generation and follow this backlog procedure without skipping any step.</critical>
  <critical>This backlog phase transforms strategic functional requirements into bite-sized stories for development agents.</critical>
  <critical>Every story must be completable by a single dev agent in one focused session.</critical>
  <critical>This is a living document that evolves through later UX and Architecture workflows.</critical>
  <critical>Communicate all responses in {communication_language} and generate backlog content in {document_output_language}.</critical>
  <critical>Write to {{backlog_output_file}} continuously as you work, never waiting until the end.</critical>
  <critical>Input documents mirror the dedicated backlog workflow: the PRD is mandatory, domain and product briefs are optional but should be loaded when available.</critical>

  <action>Inform {user_name} that backlog creation has begun and reference {{default_output_file}} as the source of truth.</action>

  <substep n="5a" goal="Load PRD and extract requirements">
    <action>Load PRD.md (required) and attempt to load domain-brief.md and product-brief.md if present, supporting both whole and sharded documents.</action>
    <action>Emphasize that PRD functional requirements (FR1, FR2, FR3...) are flat strategic capabilities that describe WHAT, not HOW.</action>
    <action>Explain that this backlog step must map each FR to epics and stories, add implementation details, and enrich acceptance criteria.</action>
    <action>Extract from the PRD: all functional requirements, non-functional requirements, domain considerations, project type and complexity, MVP/Growth/Vision scope, technical constraints, user types, and success criteria.</action>
    <action>Produce a complete FR inventory list to ensure coverage (FR1: [description] ... FRN: [description]).</action>
    <template-output>fr_inventory</template-output>
  </substep>

  <substep n="5b" goal="Propose epic structure from natural groupings">
    <action>Identify organic epic boundaries by clustering related capabilities, journeys, business goals, compliance demands, and technical systems.</action>
    <action>Name epics based on user or business value (e.g., "User Onboarding", "Content Discovery", "Compliance Framework"). Avoid technical-layer names.</action>
    <action>Ensure each epic delivers independent value, contains 3-8 related capabilities, and can be delivered cohesively.</action>
    <action>For greenfield projects, make Epic 1 a foundation epic covering setup, infrastructure, and deployment pipelines.</action>
    <action>Map every FR from the inventory to at least one epic and highlight sequencing rationale.</action>
    <template-output>epics_summary</template-output>
    <template-output>fr_coverage_map</template-output>
  </substep>

  <substep n="5c" goal="Decompose each epic into bite-sized stories" repeat="for-each-epic">
    <action>Remind everyone of the altitude shift: PRD FRs describe strategic outcomes, while stories in this step describe tactical implementation details.</action>
    <action>Each epic MUST be decomposed story-by-story using a 3-agent relay (Product Manager → UX Designer → Strategist/Architect). This sequence is mandatory for EVERY story from Epic 1 Story 1.1 to the final story of the backlog—no skipping, even if information feels repetitive.</action>
    <action>For each epic, break work into small, vertically sliced stories with full UI, validation, performance, accessibility, and error-handling details. Epic 1 (Foundation) MUST start with project setup and environment initialization so that all later stories have a platform to build upon.</action>
    <action>Story 1.1 is ALWAYS the "Project Foundation" story dedicated to establishing the repository and the minimal scaffolding required for developers to start coding immediately (initialize the repo, define the base directory structure, add README/CONTRIBUTING, example env files, lint/format configs, baseline scripts).</action>
    <action>Each story follows this pattern:
      As a [user type],
      I want [capability],
      So that [value/benefit].</action>
    <action>Execute the following phases sequentially for every story M in epic {{N}} before moving to story M+1:</action>
    <action><strong>Phase 1 – Product Manager (Story Author):</strong>
      - Draft the complete user story text.
      - Produce at least 2 BDD acceptance criteria (Given/When/Then) covering happy path, validation failure, and edge/negative behavior.
      - Define dependencies referencing only previous stories ("Story {{N}}.{{X}}") and initial technical notes (validations, data touchpoints, instrumentation needs).
      - If any information is missing, STOP and ask {user_name} a targeted question referencing the specific story ID (e.g., "Need validation rules for Story {{N}}.{{M}}") before proceeding.</action>
    <action><strong>Phase 2 – UX Designer (Interaction Guardian):</strong>
      - Enrich acceptance criteria with interaction specifics, responsive breakpoints, accessibility requirements, and UX success metrics.
      - Document explicit test scenarios listing both positive flow and at least one edge/negative scenario with measurable outcomes.
      - Confirm prerequisites remain backward-only and that dependencies align with the user journey.
      - If UX details are unclear, pause and request clarification from {user_name} naming the exact story.</action>
    <action><strong>Phase 3 – Strategist/Architect (Quality Enforcer):</strong>
      - Validate performance targets, integrations, compliance, data contracts, and telemetry/monitoring requirements inside the technical notes.
      - Ensure the story remains vertically sliced and independently valuable.
      - If gaps persist (e.g., missing API references, security constraints), send the story back to Phase 1 with explicit questions for {user_name} and do not continue until resolved.</action>
    <action>Only after all three phases are complete may you finalize Story {{N}}.{{M}}. Then repeat the same three-phase sequence for Story {{N}}.{{M+1}}; never start the next story or epic without completing every phase for the current one.</action>
    <template-output>epic*title*{{N}}</template-output>
    <template-output>epic*goal*{{N}}</template-output>
    <action>For each story M in epic {{N}}, generate story content (user story, acceptance criteria, test scenarios, dependencies, technical notes) based on the completed phases.</action>
    <template-output>story-title-{{N}}-{{M}}</template-output>
  </substep>

  <substep n="5d" goal="Review epic breakdown and validate coverage">
    <action>Build an FR coverage matrix showing each FR mapped to its corresponding epic(s) and story(ies).</action>
    <action>Confirm that every FR from the inventory is covered, that stories remain vertically sliced, and that sequencing has no forward dependencies.</action>
    <action>Ensure acceptance criteria are testable, MVP scope is prioritized, and domain or compliance requirements are properly distributed.</action>
    <action><strong>Quality Gate – Symmetry Check:</strong> Compare the last epic and its stories to Epic 1. If ANY story lacks the 3-agent relay outputs (3+ BDD criteria, explicit test scenarios, dependencies, instrumentation-rich technical notes), immediately return to Substep 5c for that specific story and repeat the phases until the depth matches.</action>
    <action>Document any follow-up questions that were asked during this gate so {user_name} sees how gaps were resolved.</action>
    <action>Record that this backlog is the initial version and will be updated by UX and Architecture workflows before implementation.</action>
    <template-output>epic_breakdown_summary</template-output>
    <template-output>fr_coverage_matrix</template-output>
  </substep>

  <action>Summarize the backlog, highlight MVP-first ordering, and capture key insights that guide future UX and Architecture enrichment.</action>
  <action>Count total epics and total stories for reporting.</action>
  <template-output>backlog_summary</template-output>
  <template-output>epic_count</template-output>
  <template-output>story_count</template-output>
  <template-output>backlog_final_summary</template-output>

  <action>Apply backlog template: {{backlog_template}}</action>
  <action>Write complete backlog to: {{backlog_output_file}}</action>
  <action>Ensure all backlog template variables are populated and the document remains in {document_output_language}</action>
</step>

<step n="6" goal="Celebrate documentation and exit Party Mode">
  <action>Have 2-3 agents celebrate completion with characteristic responses that highlight both the PRD and the backlog.</action>
  <action>Summarize the generated artifacts and suggest logical follow-up workflows (e.g., architecture deep dive) without forcing the user to choose.</action>

  <format>
    ✅ **PRD AND PRODUCT BACKLOG COMPLETE!** ✅

    **[Icon] [Product Manager]:** [Enthusiastic acknowledgment of the discovery and backlog work]

    **[Icon] [Architect]:** [Technical validation that scope and dependencies are ready for implementation]

    **[Icon] [Strategist]:** [Strategic framing of how the backlog accelerates delivery]

    ---

    **Generated Files:**
    - PRD: {{default_output_file}}
    - Backlog: {{backlog_output_file}}

    **PRD Includes:**
    - Vision & Strategic Objectives, Business Model, Personas & Journeys, Success Criteria, Product Scope (MVP/Growth/Vision), Project Classification, Functional & Non-Functional Requirements, Technical Architecture, Epic Breakdown, Roadmap.

    **Backlog Includes:**
    - Epic proposals mapped to every FR.
    - Bite-sized user stories with BDD acceptance criteria, test scenarios, dependencies, and technical notes.
    - FR coverage matrix plus backlog summary statistics.

    Next logical steps: run the architecture-focused workflow if deeper technical decisions are needed, or move directly into implementation planning with autonomous agents.

    ---

    🎊 **Thank you for an outstanding collaborative discovery!**

    Your product {product_value_summary} now has both a comprehensive PRD and an actionable backlog.

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
