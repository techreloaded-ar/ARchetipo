/* Mockup "redesign-chat" — interazioni minime, solo per far vivere il
   prototipo. Nessuna di queste funzioni è codice destinato all'applicazione. */
(function () {
	"use strict";

	// Stessa chiave del viewer, così il mockup si apre con il tema che il
	// revisore sta già usando nell'applicazione.
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

		// L'indice si può chiudere per guadagnare larghezza; nella finestra
		// stretta lo stesso comando lo fa scorrere sopra la conversazione.
		document.querySelectorAll("[data-rail-toggle]").forEach(function (btn) {
			btn.addEventListener("click", function () {
				var shell = btn.closest(".app").querySelector(".shell");
				if (window.matchMedia("(max-width: 900px)").matches ||
					shell.closest(".narrow-frame")) {
					shell.classList.toggle("rail-open");
				} else {
					shell.classList.toggle("is-collapsed");
				}
			});
		});

		// Selezione di un thread nell'indice: qui cambia solo l'evidenza,
		// perché ogni stato del mockup è una pagina a sé.
		document.querySelectorAll(".rail-list").forEach(function (list) {
			list.querySelectorAll(".thread").forEach(function (thread) {
				thread.addEventListener("click", function () {
					list.querySelectorAll(".thread").forEach(function (other) {
						other.classList.toggle("is-current", other === thread);
					});
					var shell = thread.closest(".shell");
					if (shell) shell.classList.remove("rail-open");
				});
			});
		});

		// Il composer cresce con il testo, come ci si aspetta da una chat
		// che è la pagina e non un pannello.
		document.querySelectorAll(".composer textarea").forEach(function (ta) {
			function grow() {
				ta.style.height = "auto";
				ta.style.height = Math.min(ta.scrollHeight, 280) + "px";
			}
			ta.addEventListener("input", grow);
			grow();
		});

		// I log delle run partono in fondo: quello che conta è l'ultima riga.
		document.querySelectorAll(".log").forEach(function (log) {
			log.scrollTop = log.scrollHeight;
		});
	});
})();
