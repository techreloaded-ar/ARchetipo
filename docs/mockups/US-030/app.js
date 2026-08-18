/* Mockup US-030 — interazioni minime, solo per far vivere il prototipo.
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

		// Conferma di annullamento in due tempi (AC-5): il primo clic non
		// invia nulla, apre soltanto la domanda.
		document.querySelectorAll("[data-cancel-open]").forEach(function (btn) {
			btn.addEventListener("click", function () {
				var box = btn.closest(".run-cancel");
				if (!box) return;
				btn.classList.add("is-hidden");
				var confirm = box.querySelector(".run-cancel-confirm");
				if (confirm) confirm.classList.remove("is-hidden");
			});
		});
		document.querySelectorAll("[data-cancel-abort]").forEach(function (btn) {
			btn.addEventListener("click", function () {
				var box = btn.closest(".run-cancel");
				if (!box) return;
				var confirm = box.querySelector(".run-cancel-confirm");
				if (confirm) confirm.classList.add("is-hidden");
				var open = box.querySelector("[data-cancel-open]");
				if (open) open.classList.remove("is-hidden");
			});
		});

		// Chiusura dell'avviso di rifiuto: è non distruttivo, si limita a
		// sparire e non tocca la timeline.
		document.querySelectorAll("[data-notice-dismiss]").forEach(function (btn) {
			btn.addEventListener("click", function () {
				var notice = btn.closest(".run-notice");
				if (notice) notice.remove();
			});
		});
	});
})();
