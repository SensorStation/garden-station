const clientId = 'mqttjs_' + Math.random().toString(16).substr(2, 8);

// Derive the broker WebSocket address from the page host so this works on any device.
const host = `ws://${window.location.hostname}:8080`;
console.log('MQTT broker:', host);

const options = {
    keepalive: 60,
    clientId: clientId,
    protocolId: 'MQTT',
    protocolVersion: 4,
    clean: true,
    reconnectPeriod: 1000,
    connectTimeout: 30 * 1000,
    will: {
        topic: 'WillMsg',
        payload: 'Connection Closed abnormally..!',
        qos: 0,
        retain: false
    },
}

console.log('Connecting mqtt client');

const client = mqtt.connect(host, options);
client.on('error', (err) => {
    console.log('Connection error: ', err);
    client.end();
})

client.on('reconnect', () => {
    console.log('Reconnecting...');
})

client.on('connect', () => {
    console.log(`Client connected: ${clientId}`);
    client.subscribe('gardener/devices/env/state', (err) => {
        if (err) { console.log("subscribe error: ", err); }
    });
    client.subscribe('gardener/devices/soil/state', (err) => {
        if (err) { console.log("subscribe error: ", err); }
    });
    client.subscribe('gardener/devices/pump/state', (err) => {
        if (err) { console.log("subscribe error: ", err); }
    });
})

client.on('message', (topic, message) => {
    console.log(topic, " => ", message.toString());

    const lastPart = topic.split('/').pop();

    switch (lastPart) {
    case "state": {
        const deviceName = topic.split('/')[2];
        switch (deviceName) {
        case "env": {
            const msg = JSON.parse(message);
            document.getElementById("temperature").innerHTML = msg.temperature;
            document.getElementById("pressure").innerHTML = msg.pressure;
            document.getElementById("humidity").innerHTML = msg.humidity;
            break;
        }
        case "soil":
            document.getElementById("soil").innerHTML = message.toString();
            break;
        case "pump":
            document.getElementById("pump").innerHTML = message.toString();
            break;
        }
        break;
    }
    }
});

function On() {
    console.log("on")
    client.publish('gardener/devices/pump/set', JSON.stringify(true), { qos: 0, retain: false })
}

function Off() {
    console.log("off")
    client.publish('gardener/devices/pump/set', JSON.stringify(false), { qos: 0, retain: false })
}

document.getElementById("on").addEventListener('click', On);
document.getElementById("off").addEventListener('click', Off);
