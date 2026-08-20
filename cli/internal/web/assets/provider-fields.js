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
// (exports renderProviderFields / escapeHtml).
(function () {
	// ---- visible wording ------------------------------------------------
	// The three phrases the acceptance criteria are about, in the English of
	// every other visible string in this viewer. They are kept apart on
	// purpose: the empty entry says the provider decides, and only the catalog
	// entry the provider marked carries the words "provider default", so
	// exactly one entry of the list reads as the provider's own default.
	const EMPTY_OPTION_LABEL = "No model — the provider chooses";
	const DEFAULT_SUFFIX = " — provider default";
	const UNLISTED_SUFFIX = " — not listed";
	// The empty entry of a model option, and the sentence that takes the place
	// of the section when the selected model declares no option at all: an
	// empty container would leave the reader wondering whether the panel is
	// broken or the model simply has nothing to offer.
	const EMPTY_MODEL_OPTION_LABEL = "No value — the provider chooses";
	const NO_MODEL_OPTIONS_COPY = "This model declares no option.";

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
	 * The empty entry is always first and is what an unconfigured field
	 * selects, so leaving the model unset stays possible and still submits an
	 * empty value. A current value the catalog does not carry is kept as its
	 * own entry, selected and marked, so saving cannot silently drop it.
	 */
	function renderModelSelect(field, value, models) {
		const known = models.some((m) => String(m.id || "") === value);
		const options = [
			`<option value=""${value === "" ? " selected" : ""}>${escapeHtml(EMPTY_OPTION_LABEL)}</option>`,
		];
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
		return `<select name="provider_${escapeHtml(field.name)}">${options.join("")}</select>`;
	}

	/** One configuration field, catalog or not. */
	function renderField(provider, field, values) {
		const value = currentValue(values, field.name);
		const required = field.required
			? ' <span class="field-required">required</span>'
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
		const selected = currentValue(values, provider.model_field);
		if (selected === "") return null;
		const model = catalogOf(provider).find(
			(m) => String(m.id || "") === selected,
		);
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
	function renderModelOption(option, value) {
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
		return `<label class="field full" data-provider-field="${escapeHtml(name)}"><span>${escapeHtml(option.label || name)}</span><select name="provider_option_${escapeHtml(name)}" data-provider-option="${escapeHtml(name)}">${entries.join("")}</select>${help}</label>`;
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
			return '<p class="config-copy">This provider declares no configurable setting.</p>';
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

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = { renderProviderFields, escapeHtml };
	} else {
		window.ProviderFields = { renderProviderFields };
	}
})();
