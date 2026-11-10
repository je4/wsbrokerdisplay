const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

test('message log scroll position survives a dashboard cards swap', () => {
    const listeners = new Map();
    const elements = new Map([
        ['ntp-status-badge', {dataset: {offsetNanos: '0'}}],
        ['metric-cards-container', {id: 'metric-cards-container'}],
        ['websocket-message-log', {scrollTop: 128}],
    ]);
    const document = {
        addEventListener(name, listener) {
            listeners.set(name, listener);
        },
        getElementById(id) {
            return elements.get(id) || null;
        },
    };
    const sourcePath = path.join(__dirname, '..', 'data', 'web', 'static', 'js', 'dashboard.js');
    const source = fs.readFileSync(sourcePath, 'utf8');

    vm.runInNewContext(source, {
        Date,
        Intl,
        document,
        isNaN,
        parseInt,
        setInterval() {},
    });

    const cardsTarget = elements.get('metric-cards-container');
    listeners.get('htmx:before:swap')({detail: {target: cardsTarget}});
    elements.set('websocket-message-log', {scrollTop: 0});
    listeners.get('htmx:after:swap')({detail: {target: cardsTarget}});

    assert.equal(elements.get('websocket-message-log').scrollTop, 128);
});
