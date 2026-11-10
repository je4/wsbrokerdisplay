/**
 * WSBroker Display - Real-time Live Synchronized Clock & Telemetry Updater
 */

(function() {
    'use strict';

    let ntpOffsetNanos = 0;
    let messageLogScrollTop = null;

    function parseOffsetFromDOM() {
        const offsetEl = document.getElementById('ntp-status-badge');
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

    // Initialize clock on DOMContentLoaded
    document.addEventListener('DOMContentLoaded', function() {
        parseOffsetFromDOM();
        updateClock();
        setInterval(updateClock, 200);
    });

    // Preserve the message log scroll position across dashboard card refreshes.
    document.addEventListener('htmx:before:swap', function(event) {
        const target = event.detail && event.detail.target;
        if (!target || target.id !== 'metric-cards-container') return;

        const messageLog = document.getElementById('websocket-message-log');
        messageLogScrollTop = messageLog ? messageLog.scrollTop : null;
    });

    // Re-parse NTP offset and restore the message log position after HTMX swaps.
    document.addEventListener('htmx:after:swap', function(event) {
        parseOffsetFromDOM();

        const target = event.detail && event.detail.target;
        if (!target || target.id !== 'metric-cards-container') return;

        if (messageLogScrollTop !== null) {
            const messageLog = document.getElementById('websocket-message-log');
            if (messageLog) {
                messageLog.scrollTop = messageLogScrollTop;
            }
        }
        messageLogScrollTop = null;
    });

})();
