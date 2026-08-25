// provider-fields.js
// Pure renderer for the configuration fields an execution provider declares.
//
// The module contains no provider rules: not one field name, not one provider
// id, not one model identifier. Every word it draws comes from the provider
// view served by GET /api/execution/providers, so a field or a model it has
// never seen is rendered as it is, unchanged. In particular the module does
// not know which field a model catalog fills: the catalog field is the one the
// view names in `model_field`, and nothing else.
//
// It is pure: no DOM, no fetch, no document. It takes the provider view plus
// the current values and returns an HTML string. Wiring that string into the
// page, reading the values back, and marking a field in error belong to the
// caller.
//
// Consumable in both browser (defines window.ProviderFields) and Node
// (exports renderProviderFields / renderModelChoice / escapeHtml).
(function () {
	// ---- le parole visibili ---------------------------------------------
	// Le tre frasi di cui parlano i criteri di accettazione, nella lingua di
	// ogni altra stringa visibile di questo visore. Stanno separate apposta: la
	// voce vuota dice che decide il provider, e soltanto la voce di catalogo che
	// il provider ha marcato porta le parole "default del provider", così una
	// sola voce dell'elenco si legge come il default del provider stesso.
	const EMPTY_OPTION_LABEL = "Nessun modello — sceglie il provider";
	const DEFAULT_SUFFIX = " — default del provider";
	const UNLISTED_SUFFIX = " — non in elenco";
	// The empty entry of a model option, and the sentence that takes the place
	// of the section when the selected model declares no option at all: an
	// empty container would leave the reader wondering whether the panel is
	// broken or the model simply has nothing to offer.
	const EMPTY_MODEL_OPTION_LABEL = "Nessun valore — sceglie il provider";
	const NO_MODEL_OPTIONS_COPY = "Questo modello non dichiara nessuna opzione.";
	// The section that lets a single run depart from the configuration, and
	// the sentence that says where the model on display comes from when
	// nobody has departed from it yet.
	const NO_SETTINGS_COPY =
		"Questo provider non dichiara nessuna impostazione configurabile.";
	const MODEL_CHOICE_TITLE = "Modello per questa run";
	const INHERITED_COPY = "ereditato dal workspace";
	const WORKSPACE_SOURCE = "workspace";

	// The two naming scopes the same controls are drawn under. The prefix is
	// what the submitted name and the marker attribute are built from, so the
	// configuration form and the single-run panel can never collide even when
	// they draw the very same catalog: `provider_model` is the configured
	// value, `run_model` is the value of one run and nothing else.
	const PROVIDER_SCOPE = {
		prefix: "provider",
		selectAttrs: "",
		alwaysOfferEmpty: true,
	};
	// The run scope offers the empty entry only when the inherited model is
	// itself empty. Offering it over an inherited model would let the reader
	// pick "the provider chooses" and still watch the run start on the
	// configured model, because an empty choice is indistinguishable from no
	// choice at all on the wire: the panel would be showing one model and the
	// run using another.
	const RUN_SCOPE = {
		prefix: "run",
		selectAttrs: " data-run-model",
		alwaysOfferEmpty: false,
	};

	// ---- internal helpers ----

	/**
	 * Escape the five characters that would otherwise let payload text break
	 * out of the markup it is interpolated into. Behaviourally identical to
	 * the escapeHtml of app.js.
	 */
	function escapeHtml(s) {
		if (s === null || s === undefined) return "";
		return String(s)
			.replace(/&/g, "&amp;")
			.replace(/</g, "&lt;")
			.replace(/>/g, "&gt;")
			.replace(/"/g, "&quot;")
			.replace(/'/g, "&#39;");
	}

	/** The current value of a field as a string: absent means empty. */
	function currentValue(values, name) {
		const bag = values && typeof values === "object" ? values : {};
		const value = bag[name];
		return value === undefined || value === null ? "" : String(value);
	}

	/** The catalog entries of a view, or an empty list: partial payloads must not throw. */
	function catalogOf(provider) {
		const models = provider ? provider.models : null;
		return Array.isArray(models) ? models.filter(Boolean) : [];
	}

	/** The plain text box the form has always drawn for a configuration field. */
	function renderTextInput(field, value) {
		const inputType = field.type === "integer" ? "number" : "text";
		return `<input type="${inputType}" name="provider_${escapeHtml(field.name)}" placeholder="${escapeHtml(field.placeholder || "")}" value="${escapeHtml(value)}" />`;
	}

	/**
	 * The catalog rendered as a list of entries.
	 *
	 * The empty entry is first whenever it is offered, and is what an
	 * unconfigured field selects, so leaving the model unset stays possible and
	 * still submits an empty value. A scope that does not always offer it still
	 * gets it when the current value is empty, because that entry is then the
	 * only truthful way to show what is in force. A current value the catalog
	 * does not carry is kept as its own entry, selected and marked, so saving
	 * cannot silently drop it.
	 */
	function renderModelSelect(field, value, models, scope) {
		const naming = scope || PROVIDER_SCOPE;
		const known = models.some((m) => String(m.id || "") === value);
		const options = [];
		if (naming.alwaysOfferEmpty !== false || value === "") {
			options.push(
				`<option value=""${value === "" ? " selected" : ""}>${escapeHtml(EMPTY_OPTION_LABEL)}</option>`,
			);
		}
		if (value !== "" && !known) {
			options.push(
				`<option value="${escapeHtml(value)}" selected>${escapeHtml(value + UNLISTED_SUFFIX)}</option>`,
			);
		}
		models.forEach((model) => {
			const id = String(model.id || "");
			const text = (model.label || id) + (model.default ? DEFAULT_SUFFIX : "");
			options.push(
				`<option value="${escapeHtml(id)}"${id === value && value !== "" ? " selected" : ""}>${escapeHtml(text)}</option>`,
			);
		});
		return `<select name="${naming.prefix}_${escapeHtml(field.name)}"${naming.selectAttrs}>${options.join("")}</select>`;
	}

	/** One configuration field, catalog or not. */
	function renderField(provider, field, values) {
		const value = currentValue(values, field.name);
		const required = field.required
			? ' <span class="field-required">obbligatorio</span>'
			: "";
		const help = field.help
			? `<small class="field-help">${escapeHtml(field.help)}</small>`
			: "";
		const isCatalogField =
			!!provider.model_field && provider.model_field === field.name;
		const models = isCatalogField ? catalogOf(provider) : [];
		let control;
		let notice = "";
		if (isCatalogField && models.length) {
			control = renderModelSelect(field, value, models);
		} else {
			control = renderTextInput(field, value);
			// No catalog but a stated reason: the reader is told why the list
			// is missing and keeps typing the identifier by hand.
			if (isCatalogField && provider.models_unavailable_reason) {
				notice = `<small class="field-help field-warning">${escapeHtml(provider.models_unavailable_reason)}</small>`;
			}
		}
		return `<label class="field full" data-provider-field="${escapeHtml(field.name)}"><span>${escapeHtml(field.label || field.name)}${required}</span>${control}${notice}${help}</label>`;
	}

	/**
	 * The options declared by the model the catalog field currently holds.
	 *
	 * Returns null — meaning "there is nothing to say here" — when the provider
	 * declares no catalog field, when no model is selected, or when the
	 * selected value is not an entry of the catalog. Returns an array,
	 * possibly empty, when a catalog model really is selected: an empty array
	 * is the case the explicit sentence is about.
	 */
	function selectedModelOptions(provider, values) {
		if (!provider || !provider.model_field) return null;
		return optionsOfModel(provider, currentValue(values, provider.model_field));
	}

	/**
	 * The options declared by one catalog entry, by identifier.
	 *
	 * Same contract as selectedModelOptions and the single place that reads
	 * `options` off a catalog entry: null when there is nothing to say (no
	 * model, or a model the catalog does not carry), an array — possibly
	 * empty — when a catalog model really is named.
	 */
	function optionsOfModel(view, modelID) {
		if (modelID === "") return null;
		const model = catalogOf(view).find((m) => String(m.id || "") === modelID);
		if (!model) return null;
		return Array.isArray(model.options) ? model.options.filter(Boolean) : [];
	}

	/**
	 * One model option as a list of its declared choices.
	 *
	 * The empty entry is always first and is what an unset option selects, so
	 * leaving the decision to the provider stays possible. The label carries
	 * the same wrapper the configuration fields use, so the existing error
	 * highlighting reaches it without any change.
	 */
	function renderModelOption(option, value, scope) {
		const naming = scope || PROVIDER_SCOPE;
		const name = String(option.name || "");
		const choices = Array.isArray(option.choices)
			? option.choices.filter(Boolean)
			: [];
		const entries = [
			`<option value=""${value === "" ? " selected" : ""}>${escapeHtml(EMPTY_MODEL_OPTION_LABEL)}</option>`,
		];
		choices.forEach((choice) => {
			const choiceValue = String(choice.value || "");
			const text =
				(choice.label || choiceValue) + (choice.default ? DEFAULT_SUFFIX : "");
			entries.push(
				`<option value="${escapeHtml(choiceValue)}"${choiceValue === value && value !== "" ? " selected" : ""}>${escapeHtml(text)}</option>`,
			);
		});
		const help = option.help
			? `<small class="field-help">${escapeHtml(option.help)}</small>`
			: "";
		// The control carries the option name in an attribute of its own, and
		// not only inside the submitted name: reading it back from the prefix
		// of that name would make a configuration field called `option_x`
		// indistinguishable from the option `x`.
		return `<label class="field full" data-${naming.prefix}-field="${escapeHtml(name)}"><span>${escapeHtml(option.label || name)}</span><select name="${naming.prefix}_option_${escapeHtml(name)}" data-${naming.prefix}-option="${escapeHtml(name)}">${entries.join("")}</select>${help}</label>`;
	}

	/**
	 * The section drawn under the configuration fields for the model currently
	 * selected. Empty string when there is no model to speak about.
	 */
	function renderModelOptions(provider, values) {
		const options = selectedModelOptions(provider, values);
		if (options === null) return "";
		if (!options.length) {
			return `<p class="config-copy">${escapeHtml(NO_MODEL_OPTIONS_COPY)}</p>`;
		}
		return options
			.map((option) =>
				renderModelOption(option, currentValue(values, option.name)),
			)
			.join("");
	}

	// ---- exported API ----

	/**
	 * Render the configuration fields of a provider view.
	 *
	 * @param {object|null} provider  One entry of GET /api/execution/providers.
	 * @param {object} values         The currently configured values by field name.
	 * @returns {string}              HTML, or "" when there is no provider.
	 */
	function renderProviderFields(provider, values) {
		if (!provider || typeof provider !== "object") return "";
		const fields = Array.isArray(provider.config_fields)
			? provider.config_fields.filter(Boolean)
			: [];
		if (!fields.length) {
			return '<p class="config-copy">${escapeHtml(NO_SETTINGS_COPY)}</p>';
		}
		// The options of the selected model come after the configuration
		// fields, because they are a property of the value chosen in one of
		// them and reading them before it would be reading an answer before
		// the question.
		return (
			fields.map((f) => renderField(provider, f, values)).join("") +
			renderModelOptions(provider, values)
		);
	}

	/**
	 * Render the model — and the options of that model — a single run would
	 * use, given the view served by GET /api/execution/model-choice and the
	 * choice made in the panel so far.
	 *
	 * The view is the whole truth about what would be used and why: this
	 * function never derives inheritance from a configuration, it reads
	 * `model` and `model_source`. `selection` is what the reader has touched
	 * since the panel was opened; absent — the state a freshly opened panel is
	 * in — every control shows the inherited value.
	 *
	 * When the view is not available no `<select>` is produced at all: the
	 * effective model is stated as text and the reason is shown beside it, so
	 * the panel says the start is still possible and the choice is not.
	 *
	 * @param {object|null} view       GET /api/execution/model-choice.
	 * @param {object|null} selection  {model, options} chosen for this run.
	 * @returns {string}               HTML, or "" when there is no view.
	 */
	function renderModelChoice(view, selection) {
		if (!view || typeof view !== "object") return "";
		const chosen = selection && typeof selection === "object" ? selection : null;
		const model =
			chosen && chosen.model !== undefined && chosen.model !== null
				? String(chosen.model)
				: currentValue(view, "model");
		const optionValues =
			chosen && chosen.options && typeof chosen.options === "object"
				? chosen.options
				: view.options;
		const inherited =
			view.model_source === WORKSPACE_SOURCE
				? `<small class="field-help">${escapeHtml(INHERITED_COPY)}</small>`
				: "";
		const title = `<span>${escapeHtml(MODEL_CHOICE_TITLE)}</span>`;

		if (!view.available) {
			// The model is stated, not offered: an empty one is the provider's
			// own decision and reads with the same words the catalog entry
			// would have used.
			const effective = model === "" ? EMPTY_OPTION_LABEL : model;
			const reason = view.unavailable_reason
				? `<small class="field-help field-warning">${escapeHtml(view.unavailable_reason)}</small>`
				: "";
			return `<div class="field full" data-run-field="model">${title}<p class="config-copy">${escapeHtml(effective)}</p>${inherited}${reason}</div>`;
		}

		const control = renderModelSelect(
			{ name: "model" },
			model,
			catalogOf(view),
			RUN_SCOPE,
		);
		const field = `<label class="field full" data-run-field="model">${title}${control}${inherited}</label>`;
		// The options belong to the model currently selected, so they are
		// drawn after it and are redrawn from scratch whenever it changes.
		const options = optionsOfModel(view, model);
		if (options === null) return field;
		if (!options.length) {
			return `${field}<p class="config-copy">${escapeHtml(NO_MODEL_OPTIONS_COPY)}</p>`;
		}
		return (
			field +
			options
				.map((option) =>
					renderModelOption(
						option,
						currentValue(optionValues, String(option.name || "")),
						RUN_SCOPE,
					),
				)
				.join("")
		);
	}

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = { renderProviderFields, renderModelChoice, escapeHtml };
	} else {
		window.ProviderFields = { renderProviderFields, renderModelChoice };
	}
})();
