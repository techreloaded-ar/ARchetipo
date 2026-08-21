/* Mockup US-055 — interazioni minime, solo per far vivere il prototipo.
   Nessuna di queste funzioni è codice destinato all'applicazione reale. */
(function () {
	"use strict";

	// Tema: stessa chiave usata dal viewer, così il mockup si apre con il tema
	// che il revisore sta già usando nell'app.
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

		// I tab del dettaglio spec agganciato: sono quelli della modale, e qui
		// servono solo a mostrare che restano tutti raggiungibili (AC-5).
		document.querySelectorAll(".spec-dock").forEach(function (dock) {
			dock.querySelectorAll(".modal-tabs .tab").forEach(function (tab) {
				tab.addEventListener("click", function () {
					var name = tab.dataset.tab;
					dock.querySelectorAll(".modal-tabs .tab").forEach(function (other) {
						var on = other === tab;
						other.classList.toggle("active", on);
						other.setAttribute("aria-selected", on ? "true" : "false");
					});
					dock.querySelectorAll(".tab-panel").forEach(function (panel) {
						panel.classList.toggle("active", panel.dataset.panel === name);
						panel.classList.toggle("is-hidden", panel.dataset.panel !== name);
					});
				});
			});
		});

		// Il commutatore della finestra stretta (AC-6): un contenuto alla volta,
		// e l'altro sempre a un tocco di distanza.
		document.querySelectorAll(".lane-switch").forEach(function (bar) {
			var shell = bar.closest(".app-shell");
			if (!shell) return;
			bar.querySelectorAll(".lane-btn").forEach(function (btn) {
				btn.addEventListener("click", function () {
					var lane = btn.dataset.laneTarget;
					bar.querySelectorAll(".lane-btn").forEach(function (other) {
						other.classList.toggle("is-current", other === btn);
						other.setAttribute(
							"aria-pressed",
							other === btn ? "true" : "false",
						);
					});
					shell.querySelectorAll("[data-lane]").forEach(function (pane) {
						pane.classList.toggle("is-current-lane", pane.dataset.lane === lane);
					});
				});
			});
		});
	});
})();
