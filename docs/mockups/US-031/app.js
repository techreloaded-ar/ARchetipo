/* Mockup US-031 — interazioni minime, solo per far vivere il prototipo.
   Nessuna di queste funzioni è codice destinato all'applicazione reale. */
(function () {
	"use strict";

	// Tema: stessa chiave usata dal viewer, così il mockup si apre con il
	// tema che il revisore sta già usando nell'app.
	var KEY = "archetipo.theme";

	function currentTheme() {
		try {
			return localStorage.getItem(KEY) || "light";
		} catch (_) {
			return "light";
		}
	}

	function applyTheme(value) {
		document.documentElement.dataset.theme = value;
		try {
			localStorage.setItem(KEY, value);
		} catch (_) {}
		document.querySelectorAll("[data-theme-toggle]").forEach(function (btn) {
			btn.textContent = value === "dark" ? "tema chiaro" : "tema scuro";
		});
	}

	document.addEventListener("DOMContentLoaded", function () {
		applyTheme(currentTheme());

		document.querySelectorAll("[data-theme-toggle]").forEach(function (btn) {
			btn.addEventListener("click", function () {
				applyTheme(
					document.documentElement.dataset.theme === "dark" ? "light" : "dark",
				);
			});
		});

		// Hint dello stato "nessuna epica disponibile": il pulsante è spento,
		// il motivo si legge senza doverlo cliccare.
		document.querySelectorAll("[data-hint-toggle]").forEach(function (btn) {
			var wrap = btn.closest(".new-spec-wrap");
			if (!wrap) return;
			var hint = wrap.querySelector(".new-spec-hint");
			if (!hint) return;
			["mouseenter", "focus"].forEach(function (type) {
				btn.addEventListener(type, function () {
					hint.classList.remove("hidden");
				});
			});
			["mouseleave", "blur"].forEach(function (type) {
				btn.addEventListener(type, function () {
					hint.classList.add("hidden");
				});
			});
		});

		// Anti doppio invio (AC-4): il primo clic mette la conferma in volo e
		// spegne il bottone; ogni clic successivo non parte, e il mockup lo
		// dichiara invece di crearne una seconda.
		document.querySelectorAll("[data-submit-demo]").forEach(function (btn) {
			var form = btn.closest(".form-grid");
			var status = form ? form.querySelector("[data-submit-status]") : null;
			btn.addEventListener("click", function () {
				if (btn.disabled) return;
				btn.disabled = true;
				btn.innerHTML =
					'<span class="btn-spinner" aria-hidden="true"></span> Creazione…';
				if (form) form.classList.add("is-submitting");
				if (status) {
					status.className = "status-msg";
					status.textContent = "conferma in volo · un solo invio accettato";
				}
			});
		});

		// Passaggio dimostrativo fra i due tempi dello stato D.
		document.querySelectorAll("[data-step]").forEach(function (btn) {
			btn.addEventListener("click", function () {
				var target = btn.getAttribute("data-step");
				document.querySelectorAll("[data-step-panel]").forEach(function (panel) {
					panel.classList.toggle(
						"hidden",
						panel.getAttribute("data-step-panel") !== target,
					);
				});
				document.querySelectorAll("[data-step]").forEach(function (other) {
					other.classList.toggle(
						"current",
						other.getAttribute("data-step") === target,
					);
				});
			});
		});
	});
})();
