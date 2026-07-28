const REVIEW_PAGE_ARRAY_FIELDS = [
  "seed_reviewed_pages",
  "reviewed_pages",
  "exact_reviewed_pages",
  "review_commit_pages",
  "affected_only_reconfirmed_pages",
  "context_fresh_pages",
  "unchanged_review_metadata_pages",
  "changed_review_metadata_pages",
  "reconfirm_output_pages",
];

const REVIEW_STRING_ARRAY_FIELDS = ["output_includes"];

const DOS_DEVICE_RE = /^(?:con|prn|aux|nul|com[1-9¹²³]|lpt[1-9¹²³])(?:\..*)?$/iu;

export function portableProjectRelativePath(value, field, configPath) {
  const location = configPath ? ` in ${configPath}` : "";
  const fail = () => {
    throw new Error(`${field} must be a safe portable project-relative path${location}`);
  };

  if (typeof value !== "string" || value.trim() === "") fail();
  if (/[\u0000-\u001f\u007f]/u.test(value)) fail();

  const portable = value.replaceAll("\\", "/");
  if (portable.startsWith("/") || /^[A-Za-z]:/u.test(portable)) fail();

  const components = portable.split("/");
  if (components.some((component) => component === "" || component === "." || component === "..")) fail();
  for (const component of components) {
    if (/[<>:"|?*]/u.test(component) || /[. ]$/u.test(component) || DOS_DEVICE_RE.test(component)) fail();
  }
  return portable;
}

function requireNonEmptyString(value, field, configPath) {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`${field} must be a non-empty string in ${configPath}`);
  }
  return value;
}

function requireStringArray(value, field, configPath) {
  if (!Array.isArray(value) || !value.every((item) => typeof item === "string" && item.trim() !== "")) {
    throw new Error(`${field} must be a list of non-empty strings in ${configPath}`);
  }
  return value;
}

function normalizeReviewWiki(review, scenarioId, configPath) {
  const prefix = `scenarios.${scenarioId}.verify_review_wiki`;
  if (!review || typeof review !== "object" || Array.isArray(review)) {
    throw new Error(`${prefix} must be an object when specified in ${configPath}`);
  }

  const normalized = { ...review };
  for (const key of REVIEW_STRING_ARRAY_FIELDS) {
    if (review[key] !== undefined) {
      normalized[key] = requireStringArray(review[key], `${prefix}.${key}`, configPath);
    }
  }
  for (const key of REVIEW_PAGE_ARRAY_FIELDS) {
    if (review[key] !== undefined) {
      const values = requireStringArray(review[key], `${prefix}.${key}`, configPath);
      normalized[key] = values.map((value, index) => portableProjectRelativePath(value, `${prefix}.${key}[${index}]`, configPath));
    }
  }

  normalized.spec_code = requireNonEmptyString(review.spec_code, `${prefix}.spec_code`, configPath);
  normalized.branch = requireNonEmptyString(review.branch, `${prefix}.branch`, configPath);
  normalized.worktree = portableProjectRelativePath(review.worktree, `${prefix}.worktree`, configPath);

  if (review.wiki_root !== undefined) {
    normalized.wiki_root = portableProjectRelativePath(review.wiki_root, `${prefix}.wiki_root`, configPath);
  }
  if (review.affected_file !== undefined) {
    normalized.affected_file = portableProjectRelativePath(review.affected_file, `${prefix}.affected_file`, configPath);
  }
  if (review.seed_baseline_paths !== undefined) {
    const paths = requireStringArray(review.seed_baseline_paths, `${prefix}.seed_baseline_paths`, configPath);
    normalized.seed_baseline_paths = paths.map((value, index) => portableProjectRelativePath(value, `${prefix}.seed_baseline_paths[${index}]`, configPath));
  }

  if (typeof review.require_clean !== "boolean") {
    throw new Error(`${prefix}.require_clean must be a boolean in ${configPath}`);
  }

  if (review.implemented_file_contents !== undefined) {
    if (!review.implemented_file_contents || typeof review.implemented_file_contents !== "object" || Array.isArray(review.implemented_file_contents)) {
      throw new Error(`${prefix}.implemented_file_contents must be a path-to-content object in ${configPath}`);
    }
    normalized.implemented_file_contents = Object.create(null);
    for (const [file, content] of Object.entries(review.implemented_file_contents)) {
      if (typeof content !== "string") {
        throw new Error(`${prefix}.implemented_file_contents.${file || "(empty)"} must be a string in ${configPath}`);
      }
      const normalizedFile = portableProjectRelativePath(file, `${prefix}.implemented_file_contents key`, configPath);
      if (Object.hasOwn(normalized.implemented_file_contents, normalizedFile)) {
        throw new Error(`${prefix}.implemented_file_contents contains duplicate portable path ${normalizedFile} in ${configPath}`);
      }
      normalized.implemented_file_contents[normalizedFile] = content;
    }
  }

  return normalized;
}

export function normalizeConfig(manifest, configPath, filterScenarios) {
  const agents = manifest?.agents;
  const rawScenarios = manifest?.scenarios;

  if (!agents || typeof agents !== "object" || Object.keys(agents).length === 0) {
    throw new Error(`Missing or empty 'agents' object in ${configPath}`);
  }
  if (!rawScenarios || typeof rawScenarios !== "object" || Object.keys(rawScenarios).length === 0) {
    throw new Error(`Missing or empty 'scenarios' object in ${configPath}`);
  }

  for (const [agentId, agent] of Object.entries(agents)) {
    if (!agent || typeof agent !== "object") {
      throw new Error(`agents.${agentId} must be an object in ${configPath}`);
    }
    for (const key of ["tool", "command"]) {
      if (!agent[key] || typeof agent[key] !== "string") {
        throw new Error(`agents.${agentId}.${key} must be a non-empty string in ${configPath}`);
      }
    }
    if (!Array.isArray(agent.args) || agent.args.length === 0 || !agent.args.every((arg) => typeof arg === "string")) {
      throw new Error(`agents.${agentId}.args must be a non-empty list of strings in ${configPath}`);
    }
  }

  const scenarios = [];
  for (const [scenarioId, rawScenario] of Object.entries(rawScenarios)) {
    if (!rawScenario || typeof rawScenario !== "object") {
      throw new Error(`scenarios.${scenarioId} must be an object in ${configPath}`);
    }
    const agentId = rawScenario.agent;
    if (!agentId || typeof agentId !== "string") {
      throw new Error(`scenarios.${scenarioId}.agent must be a non-empty string referencing an agent in ${configPath}`);
    }
    const agent = agents[agentId];
    if (!agent) {
      throw new Error(`scenarios.${scenarioId} references unknown agent '${agentId}' in ${configPath}`);
    }
    const prompts = rawScenario.prompts ?? [];
    if (!Array.isArray(prompts) || !prompts.every((prompt) => typeof prompt === "string")) {
      throw new Error(`scenarios.${scenarioId}.prompts must be a list of strings when specified in ${configPath}`);
    }
    if (rawScenario.fixture !== undefined && (typeof rawScenario.fixture !== "string" || rawScenario.fixture.trim() === "")) {
      throw new Error(`scenarios.${scenarioId}.fixture must be a non-empty string when specified in ${configPath}`);
    }
    for (const key of ["archetipo_pre_commands", "archetipo_post_commands"]) {
      if (rawScenario[key] !== undefined) {
        requireStringArray(rawScenario[key], `scenarios.${scenarioId}.${key}`, configPath);
      }
    }
    if (rawScenario.verify_integrate !== undefined) {
      requireStringArray(rawScenario.verify_integrate, `scenarios.${scenarioId}.verify_integrate`, configPath);
    }
    if (rawScenario.verify_wiki_bootstrap !== undefined && (!rawScenario.verify_wiki_bootstrap || typeof rawScenario.verify_wiki_bootstrap !== "object" || Array.isArray(rawScenario.verify_wiki_bootstrap))) {
      throw new Error(`scenarios.${scenarioId}.verify_wiki_bootstrap must be an object when specified in ${configPath}`);
    }

    const verifyReviewWiki = rawScenario.verify_review_wiki === undefined
      ? undefined
      : normalizeReviewWiki(rawScenario.verify_review_wiki, scenarioId, configPath);

    scenarios.push({
      id: scenarioId,
      agentId,
      agent: { id: agentId, ...agent },
      prompts,
      env_required: rawScenario.env_required ?? agent.env_required,
      fixture: rawScenario.fixture,
      archetipo_pre_commands: rawScenario.archetipo_pre_commands ?? [],
      archetipo_post_commands: rawScenario.archetipo_post_commands ?? [],
      verify_integrate: rawScenario.verify_integrate ?? [],
      verify_wiki_bootstrap: rawScenario.verify_wiki_bootstrap,
      verify_review_wiki: verifyReviewWiki,
    });
  }

  return filterScenarioList(scenarios, filterScenarios, configPath);
}

export function filterScenarioList(scenarios, filter, configPath) {
  if (!filter) return scenarios;
  const requested = filter.split(",").map((value) => value.trim()).filter(Boolean);
  const filtered = scenarios.filter((scenario) => requested.includes(scenario.id));
  const found = new Set(filtered.map((scenario) => scenario.id));
  const missing = requested.filter((id) => !found.has(id));
  if (missing.length > 0) {
    const available = scenarios.map((scenario) => scenario.id).join(", ");
    throw new Error(`Scenario(s) not found: ${missing.join(", ")}. Available scenarios: ${available}`);
  }
  return filtered;
}
