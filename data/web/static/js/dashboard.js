/**
 * WSBroker Display - Real-time Live Synchronized Clock and Telemetry Updater
 */
(function() {
    'use strict';

    let ntpOffsetNanos = 0;

    function parseOffsetFromDOM() {
        const offsetEl = document.getElementById('ntp-offset-data');
        if (offsetEl && offsetEl.dataset.offsetNanos) {
            const nanos = parseInt(offsetEl.dataset.offsetNanos, 10);
            if (!isNaN(nanos)) {
                ntpOffsetNanos = nanos;
            }
        }
    }

    function updateClock() {
        const clockEl = document.getElementById('digital-clock');
        const dateEl = document.getElementById('digital-date');
        const tzEl = document.getElementById('digital-timezone');

        if (!clockEl) return;

        // Calculate synchronized time = local time + NTP offset
        const nowLocal = Date.now();
        const offsetMillis = Math.round(ntpOffsetNanos / 1000000);
        const syncDate = new Date(nowLocal + offsetMillis);

        const hours = String(syncDate.getHours()).padStart(2, '0');
        const minutes = String(syncDate.getMinutes()).padStart(2, '0');
        const seconds = String(syncDate.getSeconds()).padStart(2, '0');

        clockEl.textContent = `${hours}:${minutes}:${seconds}`;

        if (dateEl) {
            const options = { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' };
            dateEl.textContent = syncDate.toLocaleDateString(undefined, options);
        }

        if (tzEl) {
            const tzName = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
            const offsetHours = -syncDate.getTimezoneOffset() / 60;
            const sign = offsetHours >= 0 ? '+' : '';
            tzEl.textContent = `${tzName} (UTC${sign}${offsetHours})`;
        }
    }

    // Initialize offset from DOM
    document.addEventListener('DOMContentLoaded', function() {
        parseOffsetFromDOM();
        updateClock();
        setInterval(updateClock, 200);
    });

    // Re-parse NTP offset when HTMX swaps content
    document.addEventListener('htmx:afterSwap', function(evt) {
        parseOffsetFromDOM();
    });

})();
