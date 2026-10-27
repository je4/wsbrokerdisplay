/**
 * Automatic Dark / Light Mode Detection for WSBroker Display (FOUC Prevention).
 * Automatically synchronizes data-theme ('dark' vs 'light') with OS/browser color scheme preferences.
 */
(function() {
    'use strict';

    function syncTheme() {
        var isDark = Boolean(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches);
        var activeTheme = isDark ? 'dark' : 'light';

        if (isDark) {
            document.documentElement.classList.add('dark');
            document.documentElement.classList.remove('light');
        } else {
            document.documentElement.classList.add('light');
            document.documentElement.classList.remove('dark');
        }

        document.documentElement.setAttribute('data-theme', activeTheme);
    }

    syncTheme();

    if (window.matchMedia) {
        try {
            window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', syncTheme);
        } catch (e) {
            try {
                window.matchMedia('(prefers-color-scheme: dark)').addListener(syncTheme);
            } catch (e2) {}
        }
    }
})();
