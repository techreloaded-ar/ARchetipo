/* Mockup US-053 — interazioni minime, solo per far vivere il prototipo.
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

		// Chiusura della conversazione in due tempi (AC-6): il primo clic non
		// libera il provider, apre soltanto la domanda.
		document.querySelectorAll("[data-close-open]").forEach(function (btn) {
			btn.addEventListener("click", function () {
				var box = btn.closest(".chat-close");
				if (!box) return;
				btn.classList.add("is-hidden");
				var confirm = box.querySelector(".chat-close-confirm");
				if (confirm) confirm.classList.remove("is-hidden");
			});
		});
		document.querySelectorAll("[data-close-abort]").forEach(function (btn) {
			btn.addEventListener("click", function () {
				var box = btn.closest(".chat-close");
				if (!box) return;
				var confirm = box.querySelector(".chat-close-confirm");
				if (confirm) confirm.classList.add("is-hidden");
				var open = box.querySelector("[data-close-open]");
				if (open) open.classList.remove("is-hidden");
			});
		});
	});
})();
