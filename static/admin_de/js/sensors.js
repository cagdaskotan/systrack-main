const sensorsCharts = {
  temperature: null,
  humidity: null,
  air: null,
  pressure: null
};

var _sensorHourlyData = [];

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

/* -----------------------------
   Arc Gauge — oluştur veya güncelle (destroy yok, animasyonlu update)
----------------------------- */
function _buildGaugeOptions() {
  return {
    responsive: true,
    maintainAspectRatio: false,
    cutout: '80%',
    rotation: 225,
    circumference: 270,
    animation: { duration: 700, easing: 'easeOutQuart' },
    layout: { padding: { left: 12, right: 12, top: 12, bottom: 8 } },
    plugins: { legend: { display: false }, tooltip: { enabled: false } }
  };
}

function _makeGradientFactory(gradientStops, canvas) {
  return (ctx) => {
    const chart = ctx.chart;
    const chartArea = chart?.chartArea;
    const left  = chartArea?.left  ?? 0;
    const right = chartArea?.right ?? (chart?.width ?? canvas.width ?? 300);
    const g = chart.ctx.createLinearGradient(left, 0, right, 0);
    gradientStops.forEach(stop => g.addColorStop(stop.offset, stop.color));
    return g;
  };
}

function createArcGaugeChart(canvas, { value, min, max, gradientStops,
    trackColor = 'rgba(148,163,184,0.14)' }) {
  if (typeof Chart === 'undefined' || !canvas) return null;
  const clamped = clamp(value, min, max);
  const gradFn  = _makeGradientFactory(gradientStops, canvas);
  return new Chart(canvas.getContext('2d'), {
    type: 'doughnut',
    data: {
      datasets: [{
        data: [clamped - min, Math.max(0, max - clamped)],
        backgroundColor: (ctx) => [gradFn(ctx), trackColor],
        borderWidth: 0, borderRadius: 999, hoverOffset: 0
      }]
    },
    options: _buildGaugeOptions()
  });
}

function _updateGaugeChart(chart, canvas, value) {
  if (!chart || !canvas) return;
  const min     = parseFloat(canvas.dataset.min || '0');
  const max     = parseFloat(canvas.dataset.max || '100');
  const clamped = clamp(value, min, max);
  chart.data.datasets[0].data = [clamped - min, Math.max(0, max - clamped)];
  chart.update();
}

function renderSensorGauges() {
  if (typeof Chart === 'undefined') return;

  const humidityCanvas = document.getElementById('humidityGauge');
  if (humidityCanvas && !sensorsCharts.humidity) {
    const min   = parseFloat(humidityCanvas.dataset.min || '0');
    const max   = parseFloat(humidityCanvas.dataset.max || '100');
    const value = parseFloat(humidityCanvas.dataset.value || '0');
    sensorsCharts.humidity = createArcGaugeChart(humidityCanvas, {
      value, min, max,
      gradientStops: [
        { offset: 0.0, color: '#38bdf8' },
        { offset: 1.0, color: '#2563eb' }
      ]
    });
  }

  const airCanvas = document.getElementById('airGauge');
  if (airCanvas && !sensorsCharts.air) {
    const min   = parseFloat(airCanvas.dataset.min || '0');
    const max   = parseFloat(airCanvas.dataset.max || '1000');
    const value = parseFloat(airCanvas.dataset.value || '0');
    sensorsCharts.air = createArcGaugeChart(airCanvas, {
      value, min, max,
      gradientStops: [
        { offset: 0.00, color: '#22c55e' },
        { offset: 0.45, color: '#fbbf24' },
        { offset: 0.75, color: '#fb923c' },
        { offset: 1.00, color: '#f43f5e' }
      ]
    });
  }

  const pressureCanvas = document.getElementById('pressureGauge');
  if (pressureCanvas && !sensorsCharts.pressure) {
    const min   = parseFloat(pressureCanvas.dataset.min || '950');
    const max   = parseFloat(pressureCanvas.dataset.max || '1050');
    const value = parseFloat(pressureCanvas.dataset.value || '0');
    sensorsCharts.pressure = createArcGaugeChart(pressureCanvas, {
      value, min, max,
      gradientStops: [
        { offset: 0.0, color: '#2dd4bf' },
        { offset: 1.0, color: '#0d9488' }
      ]
    });
  }
}

function _updateGaugeLabel(labelId, value, unit, hasData) {
  const label = document.getElementById(labelId);
  if (!label) return;
  label.textContent = hasData ? `${value}${unit}`.trim() : '—';
}

function initSensorsPage() {
  if (typeof document === 'undefined') return;

  initSensorLiveUpdates();
  initSensorMQTT();
  loadSensorHourlyData();

  requestAnimationFrame(() => {
    const view = document.getElementById('sensors-view');
    if (!view) return;

    renderSensorThermometers();
    renderSensorGauges();
    renderSensorCharts();

    setTimeout(() => {
      renderSensorGauges();
      if (sensorsCharts.temperature) sensorsCharts.temperature.resize();
    }, 120);
  });
}

function loadSensorHourlyData() {
  fetch('/api/sensor-hourly')
    .then(function(r) { return r.json(); })
    .then(function(data) {
      _sensorHourlyData = Array.isArray(data) ? data : [];
      renderSensorCharts();
    })
    .catch(function(err) {
      console.error('[Sensor] hourly data error:', err);
    });
}

/* -----------------------------
   Thermometer
----------------------------- */
function renderSensorThermometers() {
  const thermometers = document.querySelectorAll('#sensors-view [data-thermo]');
  thermometers.forEach(node => {
    const value = parseFloat(node.dataset.value || '0');
    const min   = parseFloat(node.dataset.min   || '0');
    const max   = parseFloat(node.dataset.max   || '100');
    const unit  = node.dataset.unit || '';

    const fill    = node.querySelector('.thermo-fill');
    const valEl   = node.querySelector('.thermo-val');
    const tube    = node.querySelector('.thermo-tube');
    if (!fill) return;

    const percentage = (max === min) ? 0 : clamp((value - min) / (max - min), 0, 1);
    fill.style.height = `${(percentage * 100).toFixed(1)}%`;

    if (valEl) {
      valEl.textContent = node.dataset.hasData
        ? `${value.toFixed(1)} ${unit}`.trim() : '—';
      const tubeH = tube ? tube.offsetHeight : 156;
      const topPx = Math.round(tubeH * (1 - percentage));
      valEl.style.top = `${clamp(topPx, 0, tubeH - 8)}px`;
    }
  });
}

/* -----------------------------
   Chart.js (Temperature — 24 saatlik)
----------------------------- */
function bindAutoResize(chart, container) {
  if (!container) return null;
  if (typeof ResizeObserver !== 'undefined') {
    const ro = new ResizeObserver(() => chart.resize());
    ro.observe(container);
    return ro;
  }
  const onResize = () => chart.resize();
  window.addEventListener('resize', onResize);
  return { disconnect: () => window.removeEventListener('resize', onResize) };
}

function renderSensorCharts() {
  if (typeof Chart === 'undefined') return;

  Chart.defaults.font.family = '"Space Grotesk", "Inter", system-ui, sans-serif';
  Chart.defaults.color = '#94a3b8';

  const tempCanvas = document.getElementById('sensorTemperatureChart');
  if (!tempCanvas) return;

  const labels      = _sensorHourlyData.map(p => p.hour);
  const ambientData = _sensorHourlyData.map(p => p.temperature    != null ? Number(p.temperature.toFixed(1))     : null);
  const cpuData     = _sensorHourlyData.map(p => p.cpu_temperature != null ? Number(p.cpu_temperature.toFixed(1)) : null);

  if (sensorsCharts.temperature) {
    const ch = sensorsCharts.temperature;
    ch.data.labels           = labels;
    ch.data.datasets[0].data = ambientData;
    ch.data.datasets[1].data = cpuData;
    ch.update('active');
    return;
  }

  const ctx = tempCanvas.getContext('2d');
  if (!ctx) return;

  sensorsCharts.temperature = new Chart(ctx, {
    type: 'line',
    data: {
      labels,
      datasets: [
        {
          label: 'Ortam',
          data: ambientData,
          borderColor: '#38bdf8',
          backgroundColor: 'rgba(56,189,248,0.25)',
          tension: 0.45, fill: true,
          pointRadius: 3, pointBackgroundColor: '#38bdf8',
          borderWidth: 2, spanGaps: true
        },
        {
          label: 'CPU',
          data: cpuData,
          borderColor: '#fb7185',
          backgroundColor: 'rgba(251,113,133,0.25)',
          tension: 0.45, fill: true,
          pointRadius: 3, pointBackgroundColor: '#fb7185',
          borderWidth: 2, spanGaps: true
        }
      ]
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      animation: { duration: 450 },
      interaction: { mode: 'index', intersect: false },
      plugins: {
        legend: { display: false },
        tooltip: {
          backgroundColor: 'rgba(15,23,42,0.92)',
          borderColor: 'rgba(148,163,184,0.20)',
          borderWidth: 1, padding: 10
        }
      },
      scales: {
        x: { ticks: { color: '#94a3b8' }, grid: { display: false } },
        y: {
          beginAtZero: false,
          ticks: { color: '#94a3b8', callback: (val) => `${val}°C` },
          grid: { color: 'rgba(148,163,184,0.18)', borderDash: [4, 4] }
        }
      }
    }
  });

  const container = tempCanvas.parentElement;
  sensorsCharts.temperature.__ro = bindAutoResize(sensorsCharts.temperature, container);
  requestAnimationFrame(() => sensorsCharts.temperature.resize());
}

/* -----------------------------
   Helpers
----------------------------- */
function setGaugeValue(id, value) {
  const canvas = document.getElementById(id);
  if (!canvas) return;
  canvas.dataset.value = String(value);
  const keyMap = { humidityGauge: 'humidity', airGauge: 'air', pressureGauge: 'pressure' };
  const key = keyMap[id];
  if (key && sensorsCharts[key]) {
    _updateGaugeChart(sensorsCharts[key], canvas, parseFloat(value));
  } else {
    renderSensorGauges();
  }
}

function setThermoValue(theme, value) {
  const node = document.querySelector(`#sensors-view [data-thermo][data-theme="${theme}"]`);
  if (!node) return;
  node.dataset.value   = String(value);
  node.dataset.hasData = '1';
  renderSensorThermometers();
}

/* -----------------------------
   UI Güncelleyici (sensor-data custom event)
   Browser-side MQTT'den gelir.
----------------------------- */
function initSensorLiveUpdates() {
  window.addEventListener('sensor-data', function(e) {
    const d = e.detail;
    if (!d) return;

    const badge       = document.querySelector('#sensors-view .animate-pulse');
    const badgeParent = badge ? badge.parentElement : null;
    if (badgeParent) {
      if (d.online) {
        badgeParent.className = badgeParent.className.replace(/\b(red|gray)\b/g, 'emerald');
      } else {
        badgeParent.className = badgeParent.className.replace(/\bemerald\b/g, 'gray');
      }
    }

    if (d.temperature != null) setThermoValue('ambient', d.temperature);
    if (d.humidity != null) {
      const c = document.getElementById('humidityGauge');
      if (c) { c.dataset.hasData = '1'; _updateGaugeLabel('humidityValueLabel', d.humidity.toFixed(1), '', true); }
      setGaugeValue('humidityGauge', d.humidity);
    }
    if (d.gas != null) {
      const c = document.getElementById('airGauge');
      if (c) { c.dataset.hasData = '1'; _updateGaugeLabel('airValueLabel', d.gas.toFixed(0), '', true); }
      setGaugeValue('airGauge', d.gas);
    }
    if (d.pressure != null) {
      const c = document.getElementById('pressureGauge');
      if (c) { c.dataset.hasData = '1'; _updateGaugeLabel('pressureValueLabel', d.pressure.toFixed(1), '', true); }
      setGaugeValue('pressureGauge', d.pressure);
    }
  });

  // CPU sıcaklığı — WebSocket server-stats
  window.addEventListener('server-stats', function(e) {
    const stats = e.detail;
    if (stats && stats.temperature != null && stats.temperature > 0) {
      setThermoValue('cabinet', stats.temperature);
    }
  });

  // Her saat başında grafik verisini yenile
  (function scheduleHourlyRefresh() {
    const now  = Date.now();
    const next = Math.ceil(now / 3600000) * 3600000;
    setTimeout(function() {
      loadSensorHourlyData();
      scheduleHourlyRefresh();
    }, next - now + 2000);
  })();
}

/* -----------------------------
   Browser-side MQTT
----------------------------- */
var _sensorMqttClient = null;

function initSensorMQTT() {
  fetch('/api/sensor-config')
    .then(function(r) { return r.json(); })
    .then(function(cfg) {
      if (!cfg.mqtt_broker_url) return;
      if (!cfg.sensor_serial) { setTimeout(initSensorMQTT, 30000); return; }
      _loadMqttJs(function() { _connectSensorMQTT(cfg.mqtt_broker_url, cfg.sensor_serial, cfg.mqtt_username, cfg.mqtt_password); });
    })
    .catch(function() { setTimeout(initSensorMQTT, 10000); });
}

function _loadMqttJs(cb) {
  if (typeof mqtt !== 'undefined') { cb(); return; }
  var s = document.createElement('script');
  s.src = 'https://unpkg.com/mqtt/dist/mqtt.min.js';
  s.onload = cb;
  s.onerror = function() {
    var s2 = document.createElement('script');
    s2.src = 'https://cdn.jsdelivr.net/npm/mqtt/dist/mqtt.min.js';
    s2.onload = cb;
    document.head.appendChild(s2);
  };
  document.head.appendChild(s);
}

function _connectSensorMQTT(brokerUrl, serial, mqttUser, mqttPass) {
  if (_sensorMqttClient) { try { _sensorMqttClient.end(true); } catch(e) {} _sensorMqttClient = null; }
  var opts = { clientId: 'systrack-ui-' + Math.random().toString(16).substr(2, 8), clean: true, reconnectPeriod: 5000, connectTimeout: 10000 };
  if (mqttUser) { opts.username = mqttUser; opts.password = mqttPass; }
  var client = mqtt.connect(brokerUrl, opts);
  _sensorMqttClient = client;
  client.on('connect', function() {
    console.error('[Sensor] MQTT connected: sensors/' + serial + '/#');
    client.subscribe('sensors/' + serial + '/#');
  });
  client.on('message', function(topic, payload) {
    var parts = topic.split('/');
    if (parts.length < 3) return;
    var field = parts[2];
    var val = parseFloat(payload.toString().trim());
    if (isNaN(val)) return;
    var detail = { online: true };
    if      (field === 'temperature') detail.temperature = val;
    else if (field === 'humidity')    detail.humidity = val;
    else if (field === 'pressure')    detail.pressure = val;
    else if (field === 'gas')         detail.gas = val;
    else return;
    window.dispatchEvent(new CustomEvent('sensor-data', { detail: detail }));
  });
  client.on('offline', function() {
    window.dispatchEvent(new CustomEvent('sensor-data', { detail: { online: false } }));
  });
  client.on('error', function(err) { console.error('[Sensor] MQTT error:', err); });
}

/* -----------------------------
   Export
----------------------------- */
if (typeof window !== 'undefined') {
  window.initSensorsPage = initSensorsPage;
  window.setGaugeValue   = setGaugeValue;
  window.setThermoValue  = setThermoValue;
}
