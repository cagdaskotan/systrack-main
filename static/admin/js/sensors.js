const sensorsCharts = {
  temperature: null,
  humidity: null,
  air: null,
  pressure: null
};

// Son yüklenen saatlik veri
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

/* -----------------------------
   Gauge render (ilk oluşturma)
----------------------------- */
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

/* -----------------------------
   Gauge label güncelle
----------------------------- */
function _updateGaugeLabel(labelId, value, unit, hasData) {
  const label = document.getElementById(labelId);
  if (!label) return;
  label.textContent = hasData ? `${value}${unit}`.trim() : '—';
}

// ── State ─────────────────────────────────────────────────────────────────────
var _activeAirSensorSerial = '';
var _activeLiquidSensorSerial = '';
var _linkedSensorSerial = '';
var _allSensors         = [];
var _allAirSensors      = [];
var _allLiquidSensors   = [];
var _pollTimer          = null;
var _activeCameraSerial = '';
var _allCameras         = [];
var _cameraPollTimer    = null;
var _cameraImages       = [];
var _cameraImageIndex   = 0;
var _cameraDetailImages = [];
var _cameraDetailDate   = '';
var _cameraObjectUrls   = new Map();
var _cameraLightboxImages = [];
var _cameraLightboxIndex = 0;
var _cameraLightboxSerial = '';
var _lastSensorData = null;
var _lastLiquidSensorData = null;
var _lastLiquidHistory = [];
var _addModalType = 'sensor';

function _storageKeyForType(type) {
  if (type === 'liquid') return 'systrack_known_liquid_sensors';
  if (type === 'camera') return 'systrack_known_cameras';
  return 'systrack_known_air_sensors';
}

function _activeStorageKeyForType(type) {
  if (type === 'liquid') return 'systrack_active_liquid_sensor';
  if (type === 'camera') return 'systrack_active_camera';
  return 'systrack_active_air_sensor';
}

function _hiddenStorageKeyForType(type) {
  if (type === 'liquid') return 'systrack_hidden_liquid_sensors';
  if (type === 'camera') return 'systrack_hidden_cameras';
  return 'systrack_hidden_air_sensors';
}

function _getHiddenTypedSet(type) {
  try {
    var raw = localStorage.getItem(_hiddenStorageKeyForType(type));
    var parsed = raw ? JSON.parse(raw) : [];
    return new Set(Array.isArray(parsed) ? parsed.map(function(item) { return String(item || '').trim(); }).filter(Boolean) : []);
  } catch (_) {
    return new Set();
  }
}

function _saveHiddenTypedSet(type, set) {
  try {
    localStorage.setItem(_hiddenStorageKeyForType(type), JSON.stringify(Array.from(set || [])));
  } catch (_) {}
}

function _hideEnvironmentItem(type, serial) {
  var key = String(serial || '').trim();
  if (!key) return;
  var hidden = _getHiddenTypedSet(type);
  hidden.add(key);
  _saveHiddenTypedSet(type, hidden);
}

function _unhideEnvironmentItem(type, serial) {
  var key = String(serial || '').trim();
  if (!key) return;
  var hidden = _getHiddenTypedSet(type);
  if (!hidden.has(key)) return;
  hidden.delete(key);
  _saveHiddenTypedSet(type, hidden);
}

function _getStoredTypedList(type) {
  try {
    var raw = localStorage.getItem(_storageKeyForType(type));
    var parsed = raw ? JSON.parse(raw) : [];
    if ((!Array.isArray(parsed) || !parsed.length) && type === 'air') {
      var legacy = localStorage.getItem('systrack_known_sensors');
      parsed = legacy ? JSON.parse(legacy) : [];
    }
    return Array.isArray(parsed) ? parsed.filter(function(s) { return s && s.serial; }) : [];
  } catch (_) {
    return [];
  }
}

function _saveStoredTypedList(type, list) {
  try {
    localStorage.setItem(_storageKeyForType(type), JSON.stringify(list || []));
  } catch (_) {}
}

function _mergeSensorLists(primary, secondary) {
  var map = new Map();
  (primary || []).forEach(function(s) {
    if (s && s.serial) map.set(String(s.serial), Object.assign({}, s));
  });
  (secondary || []).forEach(function(s) {
    if (!s || !s.serial) return;
    var key = String(s.serial);
    map.set(key, Object.assign({}, s, map.get(key) || {}));
  });
  return Array.from(map.values());
}

function _getStoredSensors() {
  return _mergeSensorLists(
    _mergeSensorLists(_getStoredTypedList('air'), _getStoredTypedList('liquid')),
    _getStoredTypedList('camera')
  );
}

function _saveStoredSensors(list) {
  _saveStoredTypedList('air', list || []);
}

function _rememberSensor(serial, name, type) {
  if (!_activeAirSensorSerial && !_activeLiquidSensorSerial) return;
  var deviceType = type || (_addModalType === 'liquid' ? 'liquid' : 'air');
  var current = _getStoredTypedList(deviceType);
  var merged = _mergeSensorLists([{ serial: serial, name: name || '' }], current);
  _saveStoredTypedList(deviceType, merged);
}

function _rememberCamera(serial, name) {
  if (!_activeAirSensorSerial && !_activeLiquidSensorSerial) return;
  var current = _getStoredTypedList('camera');
  var merged = _mergeSensorLists([{ serial: serial, name: name || '' }], current);
  _saveStoredTypedList('camera', merged);
}

function _readStoredActiveSelection(type) {
  try {
    if (type === 'air') {
      return localStorage.getItem('systrack_active_air_sensor') || localStorage.getItem('systrack_active_sensor') || '';
    }
    return localStorage.getItem(_activeStorageKeyForType(type)) || '';
  } catch (_) {
    return '';
  }
}

function _storeActiveSelection(type, serial) {
  try {
    localStorage.setItem(_activeStorageKeyForType(type), serial || '');
    if (type === 'air') localStorage.setItem('systrack_active_sensor', serial || '');
  } catch (_) {}
}

// Sayfa yenilendiğinde daha önce seçili olan sensörü localStorage'dan geri yükler.
// Kayıtlı seri numarası hâlâ mevcut (ve gizlenmemiş) listede yer alıyorsa döner,
// aksi halde boş string döner.
function _resolveStoredActive(type, list) {
  var stored = String(_readStoredActiveSelection(type) || '').trim();
  if (!stored) return '';
  var exists = (list || []).some(function(item) {
    return String(item && item.serial || '').trim() === stored;
  });
  return exists ? stored : '';
}

function _inferSensorTypeFromState(raw) {
  if (!raw || typeof raw !== 'object') return '';
  if (raw.su_analog != null || raw.su_analog_durum != null || raw.water_analog != null || raw.liquid_analog != null) {
    return 'liquid';
  }
  if (
    raw.sicaklik != null || raw.temperature != null ||
    raw.nem != null || raw.humidity != null ||
    raw.basinc != null || raw.pressure != null ||
    raw.hava_kalitesi != null || raw.iaq != null
  ) {
    return 'air';
  }
  return '';
}

async function _hydrateSensorKinds(sensorList) {
  var airHints = new Set(_getStoredTypedList('air').map(function(item) { return String(item.serial); }));
  var liquidHints = new Set(_getStoredTypedList('liquid').map(function(item) { return String(item.serial); }));
  var out = [];

  for (var i = 0; i < (sensorList || []).length; i++) {
    var sensor = Object.assign({}, sensorList[i] || {});
    var serial = String(sensor.serial || '').trim();
    if (!serial) continue;

    if (liquidHints.has(serial)) {
      sensor.kind = 'liquid';
      out.push(sensor);
      continue;
    }
    if (airHints.has(serial) || serial === _linkedSensorSerial) {
      sensor.kind = 'air';
      out.push(sensor);
      continue;
    }

    try {
      var resp = await fetch('/api/sensors/' + encodeURIComponent(serial) + '/state');
      var state = resp.ok ? await resp.json() : null;
      sensor.kind = _inferSensorTypeFromState(state) || 'air';
    } catch (_) {
      sensor.kind = 'air';
    }
    out.push(sensor);
  }

  return out;
}

function _splitSensorsByKind() {
  var hiddenAir = _getHiddenTypedSet('air');
  var hiddenLiquid = _getHiddenTypedSet('liquid');
  _allAirSensors = _allSensors.filter(function(sensor) {
    var serial = String(sensor && sensor.serial || '').trim();
    return sensor.kind !== 'liquid' && serial && !hiddenAir.has(serial);
  });
  _allLiquidSensors = _allSensors.filter(function(sensor) {
    var serial = String(sensor && sensor.serial || '').trim();
    return sensor.kind === 'liquid' && serial && !hiddenLiquid.has(serial);
  });
}

function _getLinkElementsByType(type) {
  if (type === 'camera') {
    return {
      input: document.getElementById('cameraModalSerialInput'),
      btn: document.getElementById('sensorModalLinkBtn'),
      btnTxt: document.getElementById('cameraModalLinkBtnText'),
      spinner: document.getElementById('cameraModalLinkSpinner'),
      errEl: document.getElementById('cameraModalLinkError')
    };
  }
  if (type === 'liquid') {
    return {
      input: document.getElementById('liquidModalSerialInput'),
      btn: document.getElementById('sensorModalLinkBtn'),
      btnTxt: document.getElementById('liquidModalLinkBtnText'),
      spinner: document.getElementById('liquidModalLinkSpinner'),
      errEl: document.getElementById('liquidModalLinkError')
    };
  }
  return {
    input: document.getElementById('sensorModalSerialInput'),
    btn: document.getElementById('sensorModalLinkBtn'),
    btnTxt: document.getElementById('sensorModalLinkBtnText'),
    spinner: document.getElementById('sensorModalLinkSpinner'),
    errEl: document.getElementById('sensorModalLinkError')
  };
}

function _getSensorLinkElements() {
  var modal = document.getElementById('sensorAddModal');
  var modalOpen = modal && !modal.classList.contains('hidden');
  if (modalOpen) {
    return _getLinkElementsByType(_addModalType);
  }

  return {
    input: document.getElementById('sensorSerialInput'),
    btn: document.getElementById('sensorLinkBtn'),
    btnTxt: document.getElementById('sensorLinkBtnText'),
    spinner: document.getElementById('sensorLinkSpinner'),
    errEl: document.getElementById('sensorLinkError')
  };
}

function openSensorAddModal() {
  openAddModal('sensor');
}

function closeSensorAddModal() {
  closeAddModal();
}

function openCameraAddModal() {
  openAddModal('camera');
}

function closeCameraAddModal() {
  closeAddModal();
}

function openAddModal(type) {
  var modal = document.getElementById('sensorAddModal');
  if (!modal) return;
  switchAddModalTab(type || 'sensor');
  modal.classList.remove('hidden');
  document.body.classList.add('overflow-hidden');
  ['sensorModalLinkError', 'cameraModalLinkError', 'liquidModalLinkError'].forEach(function(id) {
    var el = document.getElementById(id);
    if (el) {
      el.textContent = '';
      el.classList.add('hidden');
    }
  });
  var input = document.getElementById(
    _addModalType === 'camera'
      ? 'cameraModalSerialInput'
      : _addModalType === 'liquid'
        ? 'liquidModalSerialInput'
        : 'sensorModalSerialInput'
  );
  if (input) setTimeout(function() { input.focus(); }, 30);
}

function closeAddModal() {
  var modal = document.getElementById('sensorAddModal');
  if (!modal) return;
  modal.classList.add('hidden');
  document.body.classList.remove('overflow-hidden');
}

function switchAddModalTab(type) {
  _addModalType = type === 'camera' || type === 'liquid' ? type : 'sensor';

  var tabs = {
    sensor: document.getElementById('addTabSensor'),
    camera: document.getElementById('addTabCamera'),
    liquid: document.getElementById('addTabLiquid')
  };
  var panels = {
    sensor: document.getElementById('addPanelSensor'),
    camera: document.getElementById('addPanelCamera'),
    liquid: document.getElementById('addPanelLiquid')
  };
  Object.keys(tabs).forEach(function(key) {
    if (tabs[key]) {
      var active = key === _addModalType;
      tabs[key].className = 'px-3 py-2.5 rounded-xl text-sm font-semibold transition flex items-center justify-center gap-2 ' +
        (active ? 'bg-white dark:bg-gray-800 text-gray-900 dark:text-white shadow-sm' : 'text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white');
    }
    if (panels[key]) panels[key].classList.toggle('hidden', key !== _addModalType);
  });

  var subtitle = document.getElementById('addModalSubtitle');
  var submit = document.getElementById('sensorModalLinkBtn');
  if (subtitle) {
    subtitle.textContent = _addModalType === 'camera'
      ? 'Kamera seri numarasını girip bu cihaza bağlayın.'
      : _addModalType === 'liquid'
        ? 'Sıvı sensörü seri numarasını girip bu cihaza bağlayın.'
        : 'Hava sensörü seri numarasını girip bu cihaza bağlayın.';
  }
  if (submit) {
    submit.classList.toggle('bg-sky-600', _addModalType === 'camera');
    submit.classList.toggle('hover:bg-sky-700', _addModalType === 'camera');
    submit.classList.toggle('bg-cyan-600', _addModalType === 'liquid');
    submit.classList.toggle('hover:bg-cyan-700', _addModalType === 'liquid');
    submit.classList.toggle('bg-indigo-600', _addModalType === 'sensor');
    submit.classList.toggle('hover:bg-indigo-700', _addModalType === 'sensor');
  }

  var sensorTxt = document.getElementById('sensorModalLinkBtnText');
  var cameraTxt = document.getElementById('cameraModalLinkBtnText');
  var liquidTxt = document.getElementById('liquidModalLinkBtnText');
  if (sensorTxt) sensorTxt.classList.toggle('hidden', _addModalType !== 'sensor');
  if (cameraTxt) cameraTxt.classList.toggle('hidden', _addModalType !== 'camera');
  if (liquidTxt) liquidTxt.classList.toggle('hidden', _addModalType !== 'liquid');
}

function submitAddModal() {
  if (_addModalType === 'camera') {
    linkCamera();
    return;
  }
  if (_addModalType === 'liquid') {
    linkSensor('liquid');
    return;
  }
  linkSensor('sensor');
}

// ── Init ──────────────────────────────────────────────────────────────────────
function initSensorsPage() {
  if (typeof document === 'undefined') return;
  initSensorLiveUpdates(); // WebSocket bonus listener
  _loadPageData();

  document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
      closeEnvironmentPicker();
      var lightbox = document.getElementById('cameraLightboxModal');
      if (lightbox && !lightbox.classList.contains('hidden')) {
        closeCameraLightbox();
        return;
      }
      closeSensorAddModal();
      closeCameraAddModal();
      closeCameraDetailModal();
      closeLiquidHistoryModal();
    }
    if (e.key === 'ArrowLeft') {
      moveCameraLightbox(-1);
    }
    if (e.key === 'ArrowRight') {
      moveCameraLightbox(1);
    }
    if (e.key === 'Enter') {
      var modal = document.getElementById('sensorAddModal');
      if (modal && !modal.classList.contains('hidden')) {
        e.preventDefault();
        submitAddModal();
      }
    }
  });

  document.addEventListener('click', function(e) {
    var panel = document.getElementById('environmentPickerPanel');
    if (!panel || panel.classList.contains('hidden')) return;
    var trigger = e.target && e.target.closest ? e.target.closest('button[onclick="toggleEnvironmentPicker()"]') : null;
    if (trigger || panel.contains(e.target)) return;
    closeEnvironmentPicker();
  });
}

function _liquidApiHeaders() {
  var headers = {};
  try {
    var token = localStorage.getItem('token');
    if (token) headers.Authorization = 'Bearer ' + token;
  } catch (_) {}
  return headers;
}

function _deleteLiquidSensorFromDB(serial) {
  if (!serial) return;
  fetch('/api/liquid-sensors/' + encodeURIComponent(serial), {
    method: 'DELETE',
    headers: _liquidApiHeaders()
  }).catch(function() {});
}

// Daha önce gizlenen (silinen) sıvı sensörlerini DB'den de temizle.
// Kullanıcı sayfadan kaldırdığında localStorage'a ekleniyor;
// bu fonksiyon sayfa yüklendiğinde onları DB'den de siler.
function _cleanupHiddenLiquidSensors() {
  var hidden = _getHiddenTypedSet('liquid');
  if (!hidden.size) return;
  hidden.forEach(function(serial) { _deleteLiquidSensorFromDB(serial); });
}

// Durum + sensör listesini paralel yükle
function _loadPageData() {
  Promise.all([
    fetch('/api/sensor-state').then(function(r) { return r.ok ? r.json() : {}; }).catch(function() { return {}; }),
    fetch('/api/sensors').then(function(r) { return r.ok ? r.json() : { sensors: [] }; }).catch(function() { return { sensors: [] }; }),
    fetch('/api/cameras').then(function(r) { return r.ok ? r.json() : { cameras: [] }; }).catch(function() { return { cameras: [] }; })
  ]).then(async function(results) {
    var stateRes = results[0] || {};
    var listRes = results[1] || {};
    var cameraRes = results[2] || {};

    _linkedSensorSerial = stateRes.sensor_serial || '';
    _allSensors = await _hydrateSensorKinds(_mergeSensorLists(listRes.sensors || [], _mergeSensorLists(_getStoredTypedList('air'), _getStoredTypedList('liquid'))));
    _splitSensorsByKind();
    _cleanupHiddenLiquidSensors();
    _allCameras = _mergeSensorLists(cameraRes.cameras || [], _getStoredTypedList('camera')).filter(function(camera) {
      var serial = String(camera && camera.serial || '').trim();
      return serial && !_getHiddenTypedSet('camera').has(serial);
    });

    // Sayfa yenilenince önceki seçimi localStorage'dan geri yükle (yoksa boş kalır).
    _activeAirSensorSerial = _resolveStoredActive('air', _allAirSensors);
    _activeLiquidSensorSerial = _resolveStoredActive('liquid', _allLiquidSensors);
    _activeCameraSerial = _resolveStoredActive('camera', _allCameras);

    _renderEnvironmentDropdowns();
    _showDataView();

    if (stateRes.data && _activeAirSensorSerial === _linkedSensorSerial) {
      var d0 = _normalizeStateFields(stateRes.data);
      if (d0) window.dispatchEvent(new CustomEvent('sensor-data', { detail: d0 }));
    }

    _startPolling();
    loadCameraPreviewData();
  });
}

function _renderEnvironmentDropdowns() {
  _renderTypedSensorDropdown('airSensorSelectDropdown', _allAirSensors, _activeAirSensorSerial, 'Air sensörü seçin');
  _renderTypedSensorDropdown('liquidSensorSelectDropdown', _allLiquidSensors, _activeLiquidSensorSerial, 'Liquid sensörü seçin');
  _renderTypedSensorDropdown('cameraSelectDropdown', _allCameras, _activeCameraSerial, 'Kamera seçin');
  _renderSensorDropdown(_allAirSensors, _activeAirSensorSerial);
}

function _renderTypedSensorDropdown(elementId, list, activeSerial, placeholder) {
  var select = document.getElementById(elementId);
  if (!select) return;

  select.innerHTML = '';
  var emptyOpt = document.createElement('option');
  emptyOpt.value = '';
  emptyOpt.textContent = placeholder;
  select.appendChild(emptyOpt);

  (list || []).forEach(function(item) {
    var opt = document.createElement('option');
    opt.value = item.serial;
    opt.textContent = item.name ? item.serial + ' — ' + item.name : item.serial;
    if (item.serial === activeSerial) opt.selected = true;
    select.appendChild(opt);
  });

  if (activeSerial) select.value = activeSerial;
  select.disabled = !list || !list.length;
}

function applyEnvironmentSelections() {
  var airSelect = document.getElementById('airSensorSelectDropdown');
  var liquidSelect = document.getElementById('liquidSensorSelectDropdown');
  var cameraSelect = document.getElementById('cameraSelectDropdown');

  _activeAirSensorSerial = airSelect ? String(airSelect.value || '').trim() : '';
  _activeLiquidSensorSerial = liquidSelect ? String(liquidSelect.value || '').trim() : '';
  _activeCameraSerial = cameraSelect ? String(cameraSelect.value || '').trim() : '';

  _storeActiveSelection('air', _activeAirSensorSerial);
  _storeActiveSelection('liquid', _activeLiquidSensorSerial);
  _storeActiveSelection('camera', _activeCameraSerial);

  _showDataView();
  _startPolling();
  loadSensorHourlyData(_activeAirSensorSerial);

  if (_activeCameraSerial) {
    loadCameraImages(_activeCameraSerial);
  } else {
    renderCameraPreview([], null);
  }
}

// ── View geçişleri ────────────────────────────────────────────────────────────
function _showDataView() {
  var setup = document.getElementById('sensor-setup-card');
  var data  = document.getElementById('sensor-data-view');
  if (setup) setup.classList.add('hidden');
  if (data)  data.classList.remove('hidden');

  // Gizliyken oluşturulmuş bozuk chart'ları temizle
  Object.keys(sensorsCharts).forEach(function(k) {
    if (sensorsCharts[k] && typeof sensorsCharts[k].destroy === 'function') {
      sensorsCharts[k].destroy();
    }
    sensorsCharts[k] = null;
  });

  if (!_activeAirSensorSerial) {
    _clearAirSensorView();
    _setStatusBadge('sensorOnlineBadge', false, 'Air Online', 'Air Eklenmedi', 'emerald');
    _toggleAirSensorEmptyState(true);
  } else {
    _toggleAirSensorEmptyState(false);
  }

  if (!_activeLiquidSensorSerial) {
    _setStatusBadge('liquidOnlineBadge', false, 'Liquid Online',
      _allLiquidSensors.length === 0 ? 'Liquid Eklenmedi' : 'Liquid Bekleniyor', 'cyan');
  }

  requestAnimationFrame(function() {
    renderSensorThermometers();
    renderSensorGauges();
    renderSensorCharts();
    loadSensorHourlyData(_activeAirSensorSerial);
    setTimeout(function() {
      renderSensorGauges();
      if (sensorsCharts.temperature) sensorsCharts.temperature.resize();
    }, 150);
  });
}

function _showSetupView() {
  _showDataView();
}

function _toggleAirSensorEmptyState(show) {
  ['temperatureEmptyState', 'humidityEmptyState', 'airGaugeEmptyState', 'pressureEmptyState'].forEach(function(id) {
    var node = document.getElementById(id);
    if (!node) return;
    node.classList.toggle('hidden', !show);
  });
}

// ── Polling — esas veri kaynağı ───────────────────────────────────────────────
function _startPolling() {
  if (_pollTimer) { clearInterval(_pollTimer); _pollTimer = null; }
  if (!_activeAirSensorSerial && !_activeLiquidSensorSerial) return;

  _pollAirOnce();
  _pollLiquidOnce();
  _pollTimer = setInterval(function() {
    _pollAirOnce();
    _pollLiquidOnce();
  }, 5000);
}

function _pollAirOnce() {
  var serial = _activeAirSensorSerial;
  if (!serial) {
    window.dispatchEvent(new CustomEvent('sensor-data', { detail: { online: false } }));
    return;
  }
  fetch('/api/sensors/' + encodeURIComponent(serial) + '/state')
    .then(function(r) { return r.ok ? r.json() : null; })
    .then(function(state) {
      if (!state) return;
      var d = _normalizeStateFields(state);
      if (d) window.dispatchEvent(new CustomEvent('sensor-data', { detail: d }));
    })
    .catch(function() { /* sessizce geç */ });
}

function _pollLiquidOnce() {
  var serial = _activeLiquidSensorSerial;
  if (!serial) {
    _applyLiquidSensorState(null);
    return;
  }

  fetch('/api/liquid-sensors/' + encodeURIComponent(serial) + '/state')
    .then(function(r) { return r.ok ? r.json() : null; })
    .then(function(state) {
      var normalized = _normalizeLiquidStateFields(serial, state);
      if (normalized && state && typeof state === 'object') {
        normalized.ipAddress = String(state.wifi_ip || state.ip || state.eth_ip || '').trim() || '—';
        normalized.macAddress = String(state.wifi_mac || state.mac || state.eth_mac || '').trim() || '—';
      }
      _applyLiquidSensorState(normalized);
    })
    .catch(function() {
      _applyLiquidSensorState({
        serial: serial,
        online: false,
        statusText: 'Bağlantı yok',
        analogValue: null,
        ipAddress: '—',
        macAddress: '—',
        connectionText: 'Sunucuya ulaşılamadı',
        updatedAtText: 'Hata',
        contactDetected: false
      });
    });
}

// Management ham field adlarını (sicaklik, nem...) sensor-data formatına çevir
function _normalizeStateFields(raw) {
  if (!raw) return null;
  function n(v) {
    if (v == null) return null;
    var x = typeof v === 'string' ? parseFloat(v) : Number(v);
    return isFinite(x) ? x : null;
  }
  var temp = n(raw.sicaklik     != null ? raw.sicaklik     : raw.temperature);
  var hum  = n(raw.nem          != null ? raw.nem          : raw.humidity);
  var pres = n(raw.basinc       != null ? raw.basinc       : raw.pressure);
  var gas  = n(raw.gaz_direnci  != null ? raw.gaz_direnci  : raw.gas);
  var iaq  = n(raw.hava_kalitesi != null ? raw.hava_kalitesi : null);

  // Hiç veri yoksa null döndür (online event dispatch etme)
  if (temp == null && hum == null && pres == null && gas == null && iaq == null) return null;
  return { online: true, temperature: temp, humidity: hum, pressure: pres, gas: gas, iaq: iaq };
}

function _normalizeLiquidStateFields(serial, raw) {
  if (!raw || typeof raw !== 'object') return null;

  function n(v) {
    if (v == null) return null;
    var x = typeof v === 'string' ? parseFloat(v) : Number(v);
    return isFinite(x) ? x : null;
  }
  function t(v) {
    return v == null ? '' : String(v).trim();
  }
  function normalizeWetness(v) {
    return /^islak$/i.test(String(v || '').trim()) ? 'ıslak' : v;
  }

  var statusText = normalizeWetness(t(raw.su_analog_durum != null ? raw.su_analog_durum : raw.water_status));
  var analogValue = n(raw.su_analog != null ? raw.su_analog : raw.water_analog);
  var connectionText = t(raw.durum || raw.status || raw.state || '');
  var ipAddress = t(raw.wifi_ip || raw.ip || raw.eth_ip || '');
  var macAddress = t(raw.wifi_mac || raw.mac || raw.eth_mac || '');
  var contactDetected = !!statusText && !/kuru|dry|normal/i.test(statusText);

  if (!statusText && analogValue == null && !connectionText && !ipAddress && !macAddress) return null;
  return {
    serial: serial,
    online: true,
    statusText: statusText || 'Bilinmiyor',
    analogValue: analogValue,
    connectionText: connectionText || 'Canlı veri alınıyor',
    updatedAtText: new Date().toLocaleString('tr-TR'),
    contactDetected: contactDetected
  };
}

function _setStatusBadge(id, online, onlineText, offlineText, color) {
  var badge = document.getElementById(id);
  if (!badge) return;
  var activeColor = color || 'emerald';
  var bgClass = online ? ('bg-' + activeColor + '-500') : 'bg-gray-400';
  var borderClass = online
    ? ('border-' + activeColor + '-200 dark:border-' + activeColor + '-500/30 bg-' + activeColor + '-50 dark:bg-' + activeColor + '-500/10 text-' + activeColor + '-700 dark:text-' + activeColor + '-300')
    : 'border-gray-200 dark:border-gray-600 bg-gray-100 dark:bg-gray-700/60 text-gray-600 dark:text-gray-300';
  badge.className = 'inline-flex items-center justify-center gap-2 px-4 py-2 rounded-xl border text-sm font-semibold ' + borderClass;
  badge.innerHTML = '<span class="h-2.5 w-2.5 rounded-full ' + bgClass + (online ? ' animate-pulse' : '') + '"></span>' + (online ? onlineText : offlineText);
}

function _applyLiquidSensorState(data) {
  _lastLiquidSensorData = data;

  var serialEl = document.getElementById('liquidSensorSerialValue');
  var updatedEl = document.getElementById('liquidSensorUpdatedAtValue');
  var statusEl = document.getElementById('liquidSensorStatusValue');
  var wetnessEl = document.getElementById('liquidSensorWetnessValue');
  var ipEl = document.getElementById('liquidSensorIpValue');
  var macEl = document.getElementById('liquidSensorMacValue');
  var analogEl = wetnessEl;
  var connectionEl = ipEl;
  var lastEventEl = document.getElementById('liquidSensorLastEventValue');
  var pillEl = document.getElementById('liquidSensorAlertPill');

  if (!data) {
    if (serialEl) serialEl.textContent = _activeLiquidSensorSerial ? _activeLiquidSensorSerial : 'Liquid sensörü seçilmedi';
    if (updatedEl) updatedEl.textContent = _activeLiquidSensorSerial ? 'Canlı veri bekleniyor.' : 'Seçim yapılmadı.';
    if (statusEl) statusEl.textContent = '—';
    if (analogEl) analogEl.textContent = '—';
    if (connectionEl) connectionEl.textContent = '—';
    if (lastEventEl) lastEventEl.textContent = 'Kayıt yok';
    if (pillEl) pillEl.className = 'inline-flex items-center gap-2 rounded-full px-3 py-1.5 text-xs font-semibold border border-gray-200 dark:border-gray-600 text-gray-600 dark:text-gray-300 bg-gray-100 dark:bg-gray-700/60';
    if (pillEl) pillEl.innerHTML = '<span class="h-2.5 w-2.5 rounded-full bg-gray-400"></span>Bekleniyor';
    _setStatusBadge('liquidOnlineBadge', false, 'Liquid Online', 'Liquid Bekleniyor', 'cyan');
    return;
  }

  if (serialEl) serialEl.textContent = data.serial || 'Liquid sensörü';
  if (updatedEl) updatedEl.textContent = data.updatedAtText || 'Az önce güncellendi';
  if (statusEl) statusEl.textContent = data.statusText || 'Bilinmiyor';
  if (wetnessEl) wetnessEl.textContent = data.statusText || 'Bilinmiyor';
  if (ipEl) ipEl.textContent = data.ipAddress || '—';
  if (macEl) macEl.textContent = data.macAddress || '—';
  if (analogEl) analogEl.textContent = data.analogValue == null ? '—' : String(Math.round(data.analogValue));
  if (connectionEl) connectionEl.textContent = data.connectionText || '—';
  if (wetnessEl) wetnessEl.textContent = data.statusText || 'Bilinmiyor';
  if (ipEl) ipEl.textContent = data.ipAddress || '—';
  if (macEl) macEl.textContent = data.macAddress || '—';
  if (pillEl) {
    if (data.contactDetected) {
      pillEl.className = 'inline-flex items-center gap-2 rounded-full px-3 py-1.5 text-xs font-semibold border border-rose-200 dark:border-rose-500/30 text-rose-700 dark:text-rose-300 bg-rose-50 dark:bg-rose-500/10';
      pillEl.innerHTML = '<span class="h-2.5 w-2.5 rounded-full bg-rose-500 animate-pulse"></span>Sıvı Teması';
    } else {
      pillEl.className = 'inline-flex items-center gap-2 rounded-full px-3 py-1.5 text-xs font-semibold border border-emerald-200 dark:border-emerald-500/30 text-emerald-700 dark:text-emerald-300 bg-emerald-50 dark:bg-emerald-500/10';
      pillEl.innerHTML = '<span class="h-2.5 w-2.5 rounded-full bg-emerald-500"></span>Normal';
    }
  }

  _setStatusBadge('liquidOnlineBadge', !!data.online, 'Liquid Online', 'Liquid Offline', 'cyan');
  loadLiquidHistorySummary();
}

// ── Dropdown ──────────────────────────────────────────────────────────────────
function _renderSensorDropdown(sensors, active) {
  var sel = document.getElementById('sensorSelectDropdown');
  if (!sel) return;

  if (sensors.length <= 1) { sel.classList.add('hidden'); return; }

  sel.innerHTML = '';
  sensors.forEach(function(s) {
    var opt = document.createElement('option');
    opt.value = s.serial;
    opt.textContent = s.name ? s.serial + ' — ' + s.name : s.serial;
    if (s.serial === active) opt.selected = true;
    sel.appendChild(opt);
  });
  sel.classList.remove('hidden');
}

// Dropdown'dan sensör geçişi — API çağrısı gerekmez, sadece polling hedefini değiştir
function switchSensor(serial) {
  if (!serial || serial === _activeAirSensorSerial) return;
  _activeAirSensorSerial = serial;
  _storeActiveSelection('air', serial);
  _showDataView();
  _startPolling();
}

// ── Sensör Ekleme ─────────────────────────────────────────────────────────────
function loadInitialSensorState() { _loadPageData(); } // eski uyumluluk

function loadSensorList() {
  fetch('/api/sensors')
    .then(function(r) { return r.ok ? r.json() : { sensors: [] }; })
    .then(async function(data) {
      _allSensors = await _hydrateSensorKinds(_mergeSensorLists(data.sensors || [], _mergeSensorLists(_getStoredTypedList('air'), _getStoredTypedList('liquid'))));
      _splitSensorsByKind();
      _renderEnvironmentDropdowns();
    })
    .catch(function() {});
}

// Yeni sensör seri numarası gir + doğrula
function linkSensor(type) {
  var sensorType = type === 'liquid' ? 'liquid' : 'sensor';
  var els     = _getSensorLinkElements();
  var input   = els.input;
  var btn     = els.btn;
  var btnTxt  = els.btnTxt;
  var spinner = els.spinner;
  var errEl   = els.errEl;

  var serial = (input ? input.value : '').trim();
  if (!serial) {
    if (errEl) { errEl.textContent = 'Lütfen bir seri numarası girin.'; errEl.classList.remove('hidden'); }
    return;
  }
  if (errEl) errEl.classList.add('hidden');
  if (btn)    btn.disabled = true;
  if (spinner) spinner.classList.remove('hidden');
  if (btnTxt) btnTxt.textContent = 'Doğrulanıyor…';

  fetch('/api/sensor-link', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sensor_serial: serial })
  })
    .then(function(r) { return r.json().then(function(d) { return { ok: r.ok, data: d }; }); })
    .then(function(res) {
      if (!res.ok) {
        var msg = (res.data && res.data.error === 'sensor_not_found')
          ? 'Bu seri numaralı sensör bulunamadı. Numarayı kontrol edin.'
          : 'Bir hata oluştu, lütfen tekrar deneyin.';
        if (errEl) { errEl.textContent = msg; errEl.classList.remove('hidden'); }
        if (btn)    btn.disabled = false;
        if (spinner) spinner.classList.add('hidden');
        if (btnTxt) btnTxt.textContent = 'Bağla';
      } else {
        if (sensorType === 'liquid') {
          _activeLiquidSensorSerial = serial;
          _storeActiveSelection('liquid', serial);
        } else {
          _linkedSensorSerial = serial;
          _activeAirSensorSerial = serial;
          _storeActiveSelection('air', serial);
        }
        _rememberSensor(serial, '', sensorType === 'liquid' ? 'liquid' : 'air');
        if (input) input.value = '';
        loadSensorList();
        _showDataView();
        _startPolling();
        closeSensorAddModal();
      }
    })
    .catch(function() {
      if (errEl) { errEl.textContent = 'Sunucuya ulaşılamadı.'; errEl.classList.remove('hidden'); }
      if (btn)    btn.disabled = false;
      if (spinner) spinner.classList.add('hidden');
      if (btnTxt) btnTxt.textContent = 'Bağla';
    });
}

/* -----------------------------
   Saatlik veri yükle + grafik güncelle
----------------------------- */
function linkCamera() {
  var input = document.getElementById('cameraModalSerialInput');
  var btn = document.getElementById('cameraModalLinkBtn') || document.getElementById('sensorModalLinkBtn');
  var btnTxt = document.getElementById('cameraModalLinkBtnText');
  var spinner = document.getElementById('cameraModalLinkSpinner');
  var errEl = document.getElementById('cameraModalLinkError');
  var serial = (input ? input.value : '').trim();

  if (!serial) {
    if (errEl) { errEl.textContent = 'Lütfen bir kamera seri numarası girin.'; errEl.classList.remove('hidden'); }
    return;
  }
  if (errEl) errEl.classList.add('hidden');
  if (btn) btn.disabled = true;
  if (spinner) spinner.classList.remove('hidden');
  if (btnTxt) btnTxt.textContent = 'Bağlanıyor...';

  fetch('/api/camera-link', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ camera_serial: serial })
  })
    .then(function(r) { return r.json().then(function(d) { return { ok: r.ok, data: d }; }); })
    .then(function(res) {
      if (!res.ok) {
        var msg = 'Kamera bağlanamadı, lütfen tekrar deneyin.';
        if (res.data && res.data.error === 'camera_not_found') {
          msg = 'Bu seri numaralı kamera bulunamadı. Önce kameradan görüntü geldiğini ve seri numarasını kontrol edin.';
        } else if (res.data && res.data.error === 'management_camera_route_missing') {
          msg = 'Yönetim sunucusunda kamera API güncellemesi aktif değil. Systrack Management backend yeniden build/deploy edilmeli.';
        }
        if (errEl) { errEl.textContent = msg; errEl.classList.remove('hidden'); }
        return;
      }
      _activeCameraSerial = serial.toUpperCase();
      _storeActiveSelection('camera', _activeCameraSerial);
      _rememberCamera(_activeCameraSerial, '');
      if (input) input.value = '';
      closeCameraAddModal();
      loadCameraPreviewData(true);
    })
    .catch(function() {
      if (errEl) { errEl.textContent = 'Sunucuya ulaşılamadı.'; errEl.classList.remove('hidden'); }
    })
    .finally(function() {
      if (btn) btn.disabled = false;
      if (spinner) spinner.classList.add('hidden');
      if (btnTxt) btnTxt.textContent = 'Bağla';
    });
}

function loadCameraPreviewData(force) {
  fetch('/api/cameras')
    .then(function(r) { return r.ok ? r.json() : { cameras: [] }; })
    .then(function(data) {
      _allCameras = _mergeSensorLists(Array.isArray(data.cameras) ? data.cameras : [], _getStoredTypedList('camera'));
      if (!_activeCameraSerial || force || !_allCameras.some(function(c) { return c.serial === _activeCameraSerial; })) {
        _activeCameraSerial = _allCameras[0] ? _allCameras[0].serial : '';
      }
      _renderEnvironmentDropdowns();
      if (!_activeCameraSerial) {
        renderCameraPreview([], null);
        return;
      }
      return loadCameraImages(_activeCameraSerial);
    })
    .catch(function() {
      renderCameraPreview([], null, 'Kamera bilgileri alınamadı.');
    });

  if (!_cameraPollTimer) {
    _cameraPollTimer = setInterval(function() {
      if (_activeCameraSerial) loadCameraImages(_activeCameraSerial);
    }, 15000);
  }
}

function loadCameraImages(serial) {
  return fetch('/api/cameras/' + encodeURIComponent(serial) + '/images?limit=10')
    .then(function(r) { return r.ok ? r.json() : { images: [] }; })
    .then(function(data) {
      renderCameraPreview(Array.isArray(data.images) ? data.images : [], serial);
    })
    .catch(function() {
      renderCameraPreview([], serial, 'Kamera görüntüleri alınamadı.');
    });
}

function renderCameraPreview(images, serial, errorText) {
  var grid = document.getElementById('cameraPreviewGrid');
  var subtitle = document.getElementById('cameraPreviewSubtitle');
  var detailBtn = document.getElementById('cameraDetailBtn');
  if (!grid) return;

  _cameraImages = images || [];
  if (_cameraImageIndex >= _cameraImages.length) _cameraImageIndex = 0;

  if (subtitle) {
    subtitle.textContent = serial ? serial : (errorText || 'Kamera eklenmedi.');
  }
  if (detailBtn) {
    detailBtn.classList.toggle('hidden', !serial);
    detailBtn.classList.toggle('inline-flex', !!serial);
  }

  if (!serial) {
    grid.className = 'grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3 min-h-[34rem] content-start';
    grid.innerHTML = cameraEmptyState(errorText || 'Kamera görüntüsü bulunamadı.', 'Kamera seri numarası ekleyerek son 10 görseli burada izleyebilirsiniz.');
    return;
  }
  if (!_cameraImages.length) {
    grid.className = 'grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3 min-h-[34rem] content-start';
    grid.innerHTML = cameraEmptyState(errorText || 'Bu kameradan henüz görüntü gelmedi.', serial);
    return;
  }

  renderCameraImageFrame();
}

function renderCameraImageFrame() {
  var grid = document.getElementById('cameraPreviewGrid');
  if (!grid || !_cameraImages.length) return;

  var img = _cameraImages[_cameraImageIndex] || _cameraImages[0];
  var url = img.image_url || '';
  var hasMultiple = _cameraImages.length > 1;
  grid.className = 'flex-1 min-h-[34rem]';
  grid.innerHTML = `
    <div class="relative bg-gray-950 rounded-2xl overflow-hidden border border-white/10 min-h-[34rem] h-full shadow-inner">
      <a href="${escapeCameraAttr(url)}" data-camera-preview="true" class="absolute inset-0 flex items-center justify-center">
        <img src="${escapeCameraAttr(url)}" alt="Kamera görüntüsü" class="relative w-full h-full object-cover">
      </a>
      ${hasMultiple ? `
        <button type="button" onclick="moveCameraImage(-1)" class="absolute left-4 top-1/2 -translate-y-1/2 w-11 h-11 rounded-full bg-white/85 hover:bg-white border border-white/40 text-gray-900 flex items-center justify-center transition shadow-lg">
          <span class="sr-only">Önceki</span>
          <i class="fas fa-chevron-left text-sm"></i>
        </button>
        <button type="button" onclick="moveCameraImage(1)" class="absolute right-4 top-1/2 -translate-y-1/2 w-11 h-11 rounded-full bg-white/85 hover:bg-white border border-white/40 text-gray-900 flex items-center justify-center transition shadow-lg">
          <span class="sr-only">Sonraki</span>
          <i class="fas fa-chevron-right text-sm"></i>
        </button>
      ` : ''}
    </div>
  `;
  bindCameraFancybox();
}

function moveCameraImage(direction) {
  if (!_cameraImages.length) return;
  _cameraImageIndex = (_cameraImageIndex + direction + _cameraImages.length) % _cameraImages.length;
  renderCameraImageFrame();
}

function bindCameraFancybox() {
  var link = document.querySelector('[data-camera-preview="true"]');
  if (!link) return;
  link.addEventListener('click', function(event) {
    if (!_cameraImages.length) return;
    event.preventDefault();
    event.stopPropagation();
    openCameraLightbox(_cameraImages, _cameraImageIndex, _activeCameraSerial || 'camera');
  });
}

function fetchCameraImageObjectUrl(serial, imageUrl, keySuffix) {
  var key = String(serial || '') + ':' + String(keySuffix || '') + ':' + String(imageUrl || '');
  if (_cameraObjectUrls.has(key)) {
    return Promise.resolve(_cameraObjectUrls.get(key));
  }

  var headers = {};
  try {
    var token = localStorage.getItem('token');
    if (token) headers.Authorization = 'Bearer ' + token;
  } catch (_) {}

  return fetch(imageUrl, {
    headers: headers,
    credentials: 'same-origin'
  })
    .then(function(res) {
      if (!res.ok) throw new Error('image fetch failed');
      return res.blob();
    })
    .then(function(blob) {
      var url = URL.createObjectURL(blob);
      _cameraObjectUrls.set(key, url);
      return url;
    });
}

function openCameraLightbox(images, startIndex, serial) {
  var modal = document.getElementById('cameraLightboxModal');
  if (!modal || !Array.isArray(images) || !images.length) return;
  if (modal.parentElement !== document.body) {
    document.body.appendChild(modal);
  }
  _cameraLightboxImages = images;
  _cameraLightboxIndex = Math.max(0, Math.min(startIndex || 0, images.length - 1));
  _cameraLightboxSerial = serial || _activeCameraSerial || 'camera';
  modal.style.position = 'fixed';
  modal.style.inset = '0';
  modal.style.width = '100vw';
  modal.style.height = '100vh';
  modal.style.zIndex = '2147483647';
  modal.classList.remove('hidden');
  document.body.classList.add('overflow-hidden');
  renderCameraLightbox();
}

function closeCameraLightbox() {
  var modal = document.getElementById('cameraLightboxModal');
  if (!modal || modal.classList.contains('hidden')) return;
  modal.classList.add('hidden');
  _cameraLightboxImages = [];
  _cameraLightboxIndex = 0;
  _cameraLightboxSerial = '';

  var detailModal = document.getElementById('cameraDetailModal');
  var addModal = document.getElementById('cameraAddModal');
  var sensorModal = document.getElementById('sensorAddModal');
  var liquidModal = document.getElementById('liquidHistoryModal');
  var hasOpenModal = (detailModal && !detailModal.classList.contains('hidden')) ||
    (addModal && !addModal.classList.contains('hidden')) ||
    (sensorModal && !sensorModal.classList.contains('hidden')) ||
    (liquidModal && !liquidModal.classList.contains('hidden'));
  if (!hasOpenModal) document.body.classList.remove('overflow-hidden');
}

function moveCameraLightbox(direction) {
  var modal = document.getElementById('cameraLightboxModal');
  if (!modal || modal.classList.contains('hidden') || !_cameraLightboxImages.length) return;
  _cameraLightboxIndex = (_cameraLightboxIndex + direction + _cameraLightboxImages.length) % _cameraLightboxImages.length;
  renderCameraLightbox();
}

function renderCameraLightbox() {
  var stage = document.getElementById('cameraLightboxStage');
  var title = document.getElementById('cameraLightboxTitle');
  var meta = document.getElementById('cameraLightboxMeta');
  var prev = document.getElementById('cameraLightboxPrev');
  var next = document.getElementById('cameraLightboxNext');
  if (!stage || !_cameraLightboxImages.length) return;

  var image = _cameraLightboxImages[_cameraLightboxIndex] || _cameraLightboxImages[0];
  var imageUrl = image.image_url || image.url || '';
  var serial = _cameraLightboxSerial || _activeCameraSerial || 'camera';
  if (title) title.textContent = serial;
  if (meta) meta.textContent = (_cameraLightboxIndex + 1) + ' / ' + _cameraLightboxImages.length;
  if (prev) prev.classList.toggle('hidden', _cameraLightboxImages.length <= 1);
  if (next) next.classList.toggle('hidden', _cameraLightboxImages.length <= 1);

  stage.innerHTML = '<div class="text-sm text-slate-400">Yükleniyor</div>';
  fetchCameraImageObjectUrl(serial, imageUrl, 'lightbox-' + _cameraLightboxIndex)
    .then(function(url) {
      stage.innerHTML = '<img src="' + escapeCameraAttr(url) + '" alt="Kamera goruntusu" class="max-w-full max-h-full object-contain rounded-lg shadow-2xl">';
    })
    .catch(function() {
      stage.innerHTML = '<img src="' + escapeCameraAttr(imageUrl) + '" alt="Kamera goruntusu" class="max-w-full max-h-full object-contain rounded-lg shadow-2xl">';
    });
}

function openCameraDetailModal() {
  if (!_activeCameraSerial) {
    openCameraAddModal();
    return;
  }

  var modal = document.getElementById('cameraDetailModal');
  var title = document.getElementById('cameraDetailTitle');
  var subtitle = document.getElementById('cameraDetailSubtitle');
  var grid = document.getElementById('cameraDetailGrid');
  var back = document.getElementById('cameraDetailBackBtn');
  if (!modal || !grid) return;

  if (title) title.textContent = _activeCameraSerial;
  if (subtitle) subtitle.textContent = 'Görüntüler yükleniyor';
  if (back) back.classList.add('hidden');
  grid.innerHTML = cameraDetailEmptyState('Yükleniyor...');
  modal.classList.remove('hidden');
  document.body.classList.add('overflow-hidden');

  fetch('/api/cameras/' + encodeURIComponent(_activeCameraSerial) + '/images?limit=2000')
    .then(function(r) { return r.ok ? r.json() : { images: [] }; })
    .then(function(data) {
      _cameraDetailImages = Array.isArray(data.images) ? data.images : [];
      showCameraDateFolders();
    })
    .catch(function() {
      _cameraDetailImages = [];
      grid.innerHTML = cameraDetailEmptyState('Görüntüler alınamadı.');
      if (subtitle) subtitle.textContent = 'Hata';
    });
}

function closeCameraDetailModal() {
  var modal = document.getElementById('cameraDetailModal');
  if (!modal) return;
  closeCameraLightbox();
  modal.classList.add('hidden');
  var liquidModal = document.getElementById('liquidHistoryModal');
  if (!(liquidModal && !liquidModal.classList.contains('hidden'))) {
    document.body.classList.remove('overflow-hidden');
  }
  _cameraDetailDate = '';
}

function showCameraDateFolders() {
  var grid = document.getElementById('cameraDetailGrid');
  var subtitle = document.getElementById('cameraDetailSubtitle');
  var back = document.getElementById('cameraDetailBackBtn');
  if (!grid) return;
  _cameraDetailDate = '';
  if (back) back.classList.add('hidden');
  if (subtitle) subtitle.textContent = _cameraDetailImages.length + ' görsel';

  var dateMap = {};
  _cameraDetailImages.forEach(function(img) {
    var date = img.date_folder || formatCameraDateOnly(img.modified_at);
    if (!dateMap[date]) dateMap[date] = 0;
    dateMap[date]++;
  });
  var dates = Object.keys(dateMap).sort().reverse();
  if (!dates.length) {
    grid.innerHTML = cameraDetailEmptyState('Görüntü bulunamadı.');
    return;
  }

  grid.className = 'grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4';
  grid.innerHTML = dates.map(function(date) {
    var displayDate = formatCameraDateDisplay(date);
    return `
      <button type="button" onclick="showCameraImagesForDate('${escapeCameraJs(date)}')" class="rounded-2xl border border-gray-200 dark:border-gray-700 bg-sky-50 hover:bg-sky-100 dark:bg-sky-500/10 dark:hover:bg-sky-500/20 text-left p-5 transition">
        <div class="flex items-center gap-3">
          <div class="w-11 h-11 rounded-xl bg-sky-100 dark:bg-sky-500/20 text-sky-600 dark:text-sky-300 flex items-center justify-center">
            <i class="fas fa-folder"></i>
          </div>
          <div>
            <p class="font-semibold text-gray-900 dark:text-white">${escapeCameraHtml(displayDate)}</p>
            <p class="text-sm text-gray-500 dark:text-gray-400 mt-0.5">${dateMap[date]} görsel</p>
          </div>
        </div>
      </button>
    `;
  }).join('');
}

async function showCameraImagesForDate(date) {
  var grid = document.getElementById('cameraDetailGrid');
  var subtitle = document.getElementById('cameraDetailSubtitle');
  var back = document.getElementById('cameraDetailBackBtn');
  if (!grid) return;

  _cameraDetailDate = date;
  var images = _cameraDetailImages.filter(function(img) {
    return (img.date_folder || formatCameraDateOnly(img.modified_at)) === date;
  });
  if (back) back.classList.remove('hidden');
  if (subtitle) subtitle.textContent = images.length + ' görsel | ' + formatCameraDateDisplay(date);
  if (!images.length) {
    grid.innerHTML = cameraDetailEmptyState('Bu tarihte görüntü yok.');
    return;
  }

  grid.className = 'grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4';
  grid.innerHTML = images.map(function(img, idx) {
    var url = img.image_url || '';
    return `
      <a href="${escapeCameraAttr(url)}" data-img-idx="${idx}" class="relative rounded-xl overflow-hidden border border-gray-200 dark:border-gray-700 bg-gray-950 aspect-video block">
        <div class="absolute inset-0 flex items-center justify-center text-xs text-gray-400">Yükleniyor</div>
      </a>
    `;
  }).join('');

  await Promise.all(images.map(function(img, idx) {
    var link = grid.querySelector('[data-img-idx="' + idx + '"]');
    if (!link) return Promise.resolve();
    return fetchCameraImageObjectUrl(_activeCameraSerial || 'camera', img.image_url || '', 'detail-' + date + '-' + idx)
      .then(function(url) {
        link.href = url;
        link.innerHTML = '<img src="' + escapeCameraAttr(url) + '" alt="' + escapeCameraAttr(img.name || 'Kamera goruntusu') + '" class="w-full h-full object-cover" loading="lazy">';
        link.addEventListener('click', function(event) {
          event.preventDefault();
          event.stopPropagation();
          openCameraLightbox(images, idx, _activeCameraSerial || 'camera');
        });
      })
      .catch(function() {
        link.innerHTML = '<div class="absolute inset-0 flex items-center justify-center text-xs text-gray-400">Acilamadi</div>';
      });
  }));

}

function cameraDetailEmptyState(text) {
  return '<div class="col-span-full min-h-[18rem] rounded-2xl border border-dashed border-gray-200 dark:border-gray-700 flex items-center justify-center text-sm text-gray-500 dark:text-gray-400">' + escapeCameraHtml(text) + '</div>';
}

function cameraEmptyState(title, subtitle) {
  return `
    <div class="sm:col-span-2 xl:col-span-3 rounded-2xl border border-dashed border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900/30 text-gray-500 dark:text-gray-400 flex flex-col items-center justify-center min-h-[34rem] text-center px-6">
      <div class="w-12 h-12 rounded-2xl bg-sky-50 dark:bg-sky-500/10 text-sky-500 dark:text-sky-300 flex items-center justify-center mb-3">
        <svg xmlns="http://www.w3.org/2000/svg" class="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8">
          <path stroke-linecap="round" stroke-linejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M4 6h9a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V8a2 2 0 012-2z"/>
        </svg>
      </div>
      <p class="text-sm font-medium">${escapeCameraHtml(title)}</p>
      <p class="text-xs mt-1">${escapeCameraHtml(subtitle || '')}</p>
    </div>
  `;
}

function formatLiquidDate(value) {
  var d = new Date(value);
  return isNaN(d.getTime()) ? '-' : d.toLocaleString('tr-TR');
}

function normalizeLiquidLabel(value) {
  var text = String(value == null ? '' : value).trim();
  return /^islak$/i.test(text) ? 'ıslak' : text;
}

function loadLiquidHistorySummary() {
  var serial = _activeLiquidSensorSerial;
  var target = document.getElementById('liquidSensorLastEventValue');
  if (!serial) {
    _lastLiquidHistory = [];
    if (target) target.textContent = 'Kayıt yok';
    return Promise.resolve();
  }

  return fetch('/api/liquid-sensors/' + encodeURIComponent(serial) + '/history?limit=5')
    .then(function(r) { return r.ok ? r.json() : { history: [] }; })
    .then(function(data) {
      _lastLiquidHistory = Array.isArray(data.history) ? data.history : [];
      if (target) {
        if (!_lastLiquidHistory.length) {
          target.textContent = 'Kayıt yok';
        } else {
          var latest = _lastLiquidHistory[0];
          latest.status_text = normalizeLiquidLabel(latest.status_text);
          target.textContent = latest.status_text + ' · ' + formatLiquidDate(latest.recorded_at);
        }
      }
    })
    .catch(function() {
      if (target) target.textContent = 'Geçmiş okunamadı';
    });
}

function openLiquidHistoryModal() {
  var modal = document.getElementById('liquidHistoryModal');
  var content = document.getElementById('liquidHistoryContent');
  var subtitle = document.getElementById('liquidHistorySubtitle');
  if (!modal || !content || !subtitle) return;

  if (!_activeLiquidSensorSerial) {
    subtitle.textContent = 'Önce bir liquid sensörü seçin.';
    content.innerHTML = '<div class="rounded-2xl border border-dashed border-gray-200 dark:border-gray-700 p-8 text-center text-sm text-gray-500 dark:text-gray-400">Geçmiş görüntülemek için liquid sensörü seçin.</div>';
    modal.classList.remove('hidden');
    document.body.classList.add('overflow-hidden');
    return;
  }

  subtitle.textContent = _activeLiquidSensorSerial + ' için kayıtlar yükleniyor...';
  content.innerHTML = '<div class="rounded-2xl border border-dashed border-gray-200 dark:border-gray-700 p-8 text-center text-sm text-gray-500 dark:text-gray-400">Yükleniyor...</div>';
  modal.classList.remove('hidden');
  document.body.classList.add('overflow-hidden');

  fetch('/api/liquid-sensors/' + encodeURIComponent(_activeLiquidSensorSerial) + '/history?limit=100')
    .then(function(r) { return r.ok ? r.json() : { history: [] }; })
    .then(function(data) {
      var history = Array.isArray(data.history) ? data.history : [];
      history = history.map(function(item) {
        return Object.assign({}, item, {
          status_text: normalizeLiquidLabel(item && item.status_text)
        });
      });
      _lastLiquidHistory = history;
      subtitle.textContent = _activeLiquidSensorSerial + ' · ' + history.length + ' kayıt';
      if (!history.length) {
        content.innerHTML = '<div class="rounded-2xl border border-dashed border-gray-200 dark:border-gray-700 p-8 text-center text-sm text-gray-500 dark:text-gray-400">Henüz kayıt yok.</div>';
        return;
      }

      content.innerHTML = history.map(function(item) {
        var contactClasses = item.contact_detected
          ? 'border-rose-200 dark:border-rose-500/30 bg-rose-50 dark:bg-rose-500/10'
          : 'border-emerald-200 dark:border-emerald-500/30 bg-emerald-50 dark:bg-emerald-500/10';
        var pillClasses = item.contact_detected
          ? 'text-rose-700 dark:text-rose-300 bg-rose-100 dark:bg-rose-500/15'
          : 'text-emerald-700 dark:text-emerald-300 bg-emerald-100 dark:bg-emerald-500/15';
        return `
          <div class="rounded-2xl border ${contactClasses} p-4">
            <div class="flex items-start justify-between gap-4">
              <div>
                <p class="text-sm font-semibold text-gray-900 dark:text-white">${escapeCameraHtml(item.status_text || 'Bilinmiyor')}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">${escapeCameraHtml(formatLiquidDate(item.recorded_at))}</p>
              </div>
              <span class="inline-flex items-center rounded-full px-3 py-1 text-xs font-semibold ${pillClasses}">
                ${item.contact_detected ? 'Temas Var' : 'Normal'}
              </span>
            </div>
            <div class="mt-3 flex flex-wrap gap-3 text-xs text-gray-600 dark:text-gray-300">
              <span class="inline-flex items-center gap-1"><i class="fas fa-hashtag text-cyan-500"></i>${escapeCameraHtml(item.sensor_serial || _activeLiquidSensorSerial)}</span>
              <span class="inline-flex items-center gap-1"><i class="fas fa-wave-square text-cyan-500"></i>${item.analog_value == null ? '—' : item.analog_value}</span>
            </div>
          </div>
        `;
      }).join('');
    })
    .catch(function() {
      subtitle.textContent = _activeLiquidSensorSerial + ' · hata';
      content.innerHTML = '<div class="rounded-2xl border border-dashed border-rose-200 dark:border-rose-500/30 p-8 text-center text-sm text-rose-500 dark:text-rose-300">Geçmiş verisi alınamadı.</div>';
    });
}

function closeLiquidHistoryModal() {
  var modal = document.getElementById('liquidHistoryModal');
  if (!modal) return;
  modal.classList.add('hidden');

  var detailModal = document.getElementById('cameraDetailModal');
  var sensorModal = document.getElementById('sensorAddModal');
  var lightbox = document.getElementById('cameraLightboxModal');
  var hasOtherModal = (detailModal && !detailModal.classList.contains('hidden')) ||
    (sensorModal && !sensorModal.classList.contains('hidden')) ||
    (lightbox && !lightbox.classList.contains('hidden'));
  if (!hasOtherModal) document.body.classList.remove('overflow-hidden');
}

function formatCameraDate(value) {
  var d = new Date(value);
  return isNaN(d.getTime()) ? '' : d.toLocaleString('tr-TR');
}

function formatCameraDateOnly(value) {
  var d = new Date(value);
  return isNaN(d.getTime()) ? '-' : d.toISOString().slice(0, 10);
}

function formatCameraDateDisplay(value) {
  var raw = String(value || '');
  var match = raw.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (match) return match[3] + '.' + match[2] + '.' + match[1];
  var d = new Date(value);
  return isNaN(d.getTime()) ? raw : d.toLocaleDateString('tr-TR');
}

function escapeCameraJs(value) {
  return String(value == null ? '' : value).replace(/\\/g, '\\\\').replace(/'/g, "\\'");
}

function escapeCameraHtml(value) {
  return String(value == null ? '' : value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function escapeCameraAttr(value) {
  return escapeCameraHtml(value);
}

function loadSensorHourlyData() {
  var serial = arguments.length > 0 ? arguments[0] : _activeAirSensorSerial;
  if (!serial) {
    _sensorHourlyData = [];
    renderSensorCharts();
    return Promise.resolve();
  }
  var url = '/api/sensor-hourly';
  if (serial) {
    url += '?serial=' + encodeURIComponent(serial);
  }

  fetch(url)
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
        ? `${value.toFixed(1)} ${unit}`.trim()
        : '—';
      // Reposition label to track fill top
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

  var chartPoints = _sensorHourlyData;
  if ((!chartPoints || !chartPoints.length) && _lastSensorData) {
    var now = new Date();
    var prev = new Date(now.getTime() - 5 * 60 * 1000);
    var temp = _lastSensorData.temperature != null ? Number(_lastSensorData.temperature) : null;
    var cpu = _lastSensorData.cpu_temperature != null ? Number(_lastSensorData.cpu_temperature) : null;
    chartPoints = [
      { hour: prev.toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit' }), temperature: temp, cpu_temperature: cpu },
      { hour: now.toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit' }), temperature: temp, cpu_temperature: cpu }
    ];
  }
  chartPoints = Array.isArray(chartPoints) ? chartPoints : [];

  const labels      = chartPoints.map(p => p.hour);
  const ambientData = chartPoints.map(p => p.temperature    != null ? Number(Number(p.temperature).toFixed(1))     : null);
  const cpuData     = chartPoints.map(p => p.cpu_temperature != null ? Number(Number(p.cpu_temperature).toFixed(1)) : null);

  if (sensorsCharts.temperature) {
    // Sadece veriyi güncelle, chart'ı destroy etme
    const ch = sensorsCharts.temperature;
    ch.data.labels                  = labels;
    ch.data.datasets[0].data        = ambientData;
    ch.data.datasets[1].data        = cpuData;
    ch.update('active');
    return;
  }

  // İlk oluşturma
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
          tension: 0.45,
          fill: true,
          pointRadius: 3,
          pointBackgroundColor: '#38bdf8',
          borderWidth: 2,
          spanGaps: true
        },
        {
          label: 'CPU',
          data: cpuData,
          borderColor: '#fb7185',
          backgroundColor: 'rgba(251,113,133,0.25)',
          tension: 0.45,
          fill: true,
          pointRadius: 3,
          pointBackgroundColor: '#fb7185',
          borderWidth: 2,
          spanGaps: true
        }
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      animation: { duration: 450 },
      interaction: { mode: 'index', intersect: false },
      plugins: {
        legend: { display: false },
        tooltip: {
          backgroundColor: 'rgba(15,23,42,0.92)',
          borderColor: 'rgba(148,163,184,0.20)',
          borderWidth: 1,
          padding: 10
        }
      },
      scales: {
        x: {
          ticks: { color: '#94a3b8' },
          grid: { display: false }
        },
        y: {
          beginAtZero: false,
          ticks: {
            color: '#94a3b8',
            callback: (val) => `${val}°C`
          },
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
function _isDataViewVisible() {
  var v = document.getElementById('sensor-data-view');
  return v && !v.classList.contains('hidden');
}

function setGaugeValue(id, value) {
  const canvas = document.getElementById(id);
  if (!canvas) return;
  canvas.dataset.value = String(value);
  if (!_isDataViewVisible()) return; // gizliyken chart oluşturma

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
  if (!_isDataViewVisible()) return; // gizliyken render etme
  renderSensorThermometers();
}

function _clearAirSensorView() {
  _lastSensorData = null;
  _sensorHourlyData = [];

  ['humidityGauge', 'airGauge', 'pressureGauge'].forEach(function(id) {
    var canvas = document.getElementById(id);
    if (canvas) {
      canvas.dataset.value = '0';
      canvas.dataset.hasData = '';
    }
  });

  _updateGaugeLabel('humidityValueLabel', '', '', false);
  _updateGaugeLabel('airValueLabel', '', '', false);
  _updateGaugeLabel('pressureValueLabel', '', '', false);

  var ambientNode = document.querySelector('#sensors-view [data-thermo][data-theme="ambient"]');
  var cpuNode = document.querySelector('#sensors-view [data-thermo][data-theme="cabinet"]');
  [ambientNode, cpuNode].forEach(function(node) {
    if (!node) return;
    node.dataset.value = '0';
    node.dataset.hasData = '';
  });

  renderSensorThermometers();
  if (sensorsCharts.humidity) _updateGaugeChart(sensorsCharts.humidity, document.getElementById('humidityGauge'), 0);
  if (sensorsCharts.air) _updateGaugeChart(sensorsCharts.air, document.getElementById('airGauge'), 0);
  if (sensorsCharts.pressure) _updateGaugeChart(sensorsCharts.pressure, document.getElementById('pressureGauge'), 0);
  renderSensorCharts();
}

/* -----------------------------
   UI Güncelleyici — sensor-data event
   (WebSocket üzerinden gelir)
----------------------------- */
function initSensorLiveUpdates() {
  window.addEventListener('sensor-data', function(e) {
    const d = e.detail;
    if (!d) return;

    if (!d.online && d.temperature == null && d.humidity == null && d.pressure == null && d.gas == null && d.iaq == null) {
      _clearAirSensorView();
      _setStatusBadge('sensorOnlineBadge', false, 'Air Online', _activeAirSensorSerial ? 'Air Bekleniyor' : 'Air Eklenmedi', 'emerald');
      _toggleAirSensorEmptyState(!_activeAirSensorSerial);
      return;
    }

    _lastSensorData = d;

    _setStatusBadge('sensorOnlineBadge', !!d.online, 'Air Online', 'Air Offline', 'emerald');
    _toggleAirSensorEmptyState(false);

    if (d.temperature != null) {
      setThermoValue('ambient', d.temperature);
    }
    if (d.humidity != null) {
      const c = document.getElementById('humidityGauge');
      if (c) c.dataset.hasData = '1';
      _updateGaugeLabel('humidityValueLabel', d.humidity.toFixed(1), '', true);
      setGaugeValue('humidityGauge', d.humidity);
    }
    // IAQ (hava_kalitesi) öncelikli; yoksa gas fallback
    var iaqVal = d.iaq != null ? d.iaq : d.gas;
    if (iaqVal != null) {
      const c = document.getElementById('airGauge');
      if (c) { c.dataset.hasData = '1'; c.dataset.max = '500'; }
      _updateGaugeLabel('airValueLabel', Math.round(iaqVal).toString(), '', true);
      setGaugeValue('airGauge', iaqVal);
    }
    if (d.pressure != null) {
      const c = document.getElementById('pressureGauge');
      if (c) c.dataset.hasData = '1';
      _updateGaugeLabel('pressureValueLabel', d.pressure.toFixed(1), '', true);
      setGaugeValue('pressureGauge', d.pressure);
    }
    renderSensorCharts();
  });

  // CPU sıcaklığı — server-stats WebSocket mesajından
  window.addEventListener('server-stats', function(e) {
    const stats = e.detail;
    if (stats && stats.temperature != null && stats.temperature > 0) {
      _lastSensorData = Object.assign({}, _lastSensorData || {}, { cpu_temperature: stats.temperature });
      setThermoValue('cabinet', stats.temperature);
      renderSensorCharts();
    }
  });

  // Her saat başında grafik verisini yenile
  (function scheduleHourlyRefresh() {
    const now  = Date.now();
    const next = Math.ceil(now / 3600000) * 3600000;
    setTimeout(function() {
      loadSensorHourlyData(_activeAirSensorSerial);
      scheduleHourlyRefresh();
    }, next - now + 2000); // +2 sn margin
  })();
}

function _escapeEnvironmentText(value) {
  return String(value == null ? '' : value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function _getEnvironmentTypeLabel(type) {
  if (type === 'liquid') return 'Sıvı sensörü';
  if (type === 'camera') return 'Kamera';
  return 'Hava sensörü';
}

function _renderEnvironmentPickerColumn(type, list, activeSerial, currentId, listId) {
  var currentEl = document.getElementById(currentId);
  var listEl = document.getElementById(listId);
  if (currentEl) currentEl.textContent = activeSerial || 'Seçilmedi';
  if (!listEl) return;

  if (!list || !list.length) {
    listEl.innerHTML =
      '<div class="rounded-2xl border border-dashed border-gray-200 dark:border-gray-600 px-3 py-4 text-sm text-gray-500 dark:text-gray-400">' +
      _getEnvironmentTypeLabel(type) + ' eklenmedi.' +
      '</div>';
    return;
  }

  listEl.innerHTML = list.map(function(item) {
    var serial = String(item && item.serial || '').trim();
    var safeSerial = serial.replace(/\\/g, '\\\\').replace(/'/g, '\\\'');
    var isActive = serial === activeSerial;
    var actionClass = isActive
      ? 'bg-emerald-600 hover:bg-emerald-700 text-white'
      : 'bg-gray-900 hover:bg-gray-800 dark:bg-white dark:hover:bg-gray-100 text-white dark:text-gray-900';
    var actionText = isActive ? 'Gösteriliyor' : 'Göster';
    var secondaryText = item && item.name ? _escapeEnvironmentText(item.name) : _getEnvironmentTypeLabel(type);

    return '' +
      '<div class="rounded-2xl border border-white/70 dark:border-gray-700 bg-white/90 dark:bg-gray-900/40 px-3 py-3 flex items-center justify-between gap-3">' +
        '<div class="min-w-0">' +
          '<p class="text-sm font-semibold text-gray-900 dark:text-white truncate">' + _escapeEnvironmentText(serial) + '</p>' +
          '<p class="text-[11px] text-gray-500 dark:text-gray-400 truncate">' + secondaryText + '</p>' +
        '</div>' +
        '<div class="flex items-center gap-2 shrink-0">' +
          '<button type="button" onclick="selectEnvironmentItem(\'' + type + '\', \'' + safeSerial + '\')" class="px-3 py-2 rounded-xl text-xs font-semibold transition ' + actionClass + '">' + actionText + '</button>' +
          '<button type="button" onclick="removeEnvironmentItem(\'' + type + '\', \'' + safeSerial + '\')" class="w-9 h-9 rounded-xl border border-rose-200 dark:border-rose-500/30 text-rose-600 dark:text-rose-300 hover:bg-rose-50 dark:hover:bg-rose-500/10 transition flex items-center justify-center" aria-label="' + _getEnvironmentTypeLabel(type) + ' sil">' +
            '<i class="fas fa-trash text-xs"></i>' +
          '</button>' +
        '</div>' +
      '</div>';
  }).join('');
}

function _renderEnvironmentDropdowns() {
  _renderEnvironmentPickerColumn('air', _allAirSensors, _activeAirSensorSerial, 'airPickerCurrent', 'airPickerList');
  _renderEnvironmentPickerColumn('liquid', _allLiquidSensors, _activeLiquidSensorSerial, 'liquidPickerCurrent', 'liquidPickerList');
  _renderEnvironmentPickerColumn('camera', _allCameras, _activeCameraSerial, 'cameraPickerCurrent', 'cameraPickerList');

  var liquidEmpty = document.getElementById('liquidEmptyState');
  var liquidContent = document.getElementById('liquidContentArea');
  var noLiquid = !_allLiquidSensors || _allLiquidSensors.length === 0;
  if (liquidEmpty) liquidEmpty.classList.toggle('hidden', !noLiquid);
  if (liquidContent) liquidContent.classList.toggle('hidden', noLiquid);
}

function toggleEnvironmentPicker() {
  var panel = document.getElementById('environmentPickerPanel');
  if (!panel) return;
  panel.classList.toggle('hidden');
}

function closeEnvironmentPicker() {
  var panel = document.getElementById('environmentPickerPanel');
  if (!panel) return;
  panel.classList.add('hidden');
}

function selectEnvironmentItem(type, serial) {
  var kind = type === 'sensor' ? 'air' : type;
  var clean = String(serial || '').trim();
  if (!clean) return;

  if (kind === 'liquid') {
    _activeLiquidSensorSerial = clean;
  } else if (kind === 'camera') {
    _activeCameraSerial = clean;
  } else {
    _activeAirSensorSerial = clean;
  }

  _storeActiveSelection(kind, clean);
  _renderEnvironmentDropdowns();
  _showDataView();
  _startPolling();

  if (kind === 'camera') {
    loadCameraImages(clean);
  }

  closeEnvironmentPicker();
}

function removeEnvironmentItem(type, serial) {
  var kind = type === 'sensor' ? 'air' : type;
  var clean = String(serial || '').trim();
  if (!clean) return;

  _hideEnvironmentItem(kind, clean);

  if (kind === 'liquid') {
    _allLiquidSensors = _allLiquidSensors.filter(function(item) { return item.serial !== clean; });
    if (_activeLiquidSensorSerial === clean) {
      _activeLiquidSensorSerial = _allLiquidSensors[0] ? _allLiquidSensors[0].serial : '';
      _storeActiveSelection('liquid', _activeLiquidSensorSerial);
      if (!_activeLiquidSensorSerial) _applyLiquidSensorState(null);
    }
    // Veritabanından da sil — bildirim dropdown'unda görünmesin
    _deleteLiquidSensorFromDB(clean);
  } else if (kind === 'camera') {
    _allCameras = _allCameras.filter(function(item) { return item.serial !== clean; });
    if (_activeCameraSerial === clean) {
      _activeCameraSerial = _allCameras[0] ? _allCameras[0].serial : '';
      _storeActiveSelection('camera', _activeCameraSerial);
      if (!_activeCameraSerial) renderCameraPreview([], null);
    }
  } else {
    _allAirSensors = _allAirSensors.filter(function(item) { return item.serial !== clean; });
    if (_activeAirSensorSerial === clean) {
      _activeAirSensorSerial = _allAirSensors[0] ? _allAirSensors[0].serial : '';
      _storeActiveSelection('air', _activeAirSensorSerial);
    }
  }

  _renderEnvironmentDropdowns();

  _showDataView();
  _startPolling();
  if (_activeCameraSerial) {
    loadCameraImages(_activeCameraSerial);
  }
}

function applyEnvironmentSelections() {
  _renderEnvironmentDropdowns();
  closeEnvironmentPicker();
}

function loadSensorList() {
  fetch('/api/sensors')
    .then(function(r) { return r.ok ? r.json() : { sensors: [] }; })
    .then(async function(data) {
      _allSensors = await _hydrateSensorKinds(_mergeSensorLists(data.sensors || [], _mergeSensorLists(_getStoredTypedList('air'), _getStoredTypedList('liquid'))));
      _splitSensorsByKind();
      _renderEnvironmentDropdowns();
    })
    .catch(function() {});
}

function linkSensor(type) {
  var sensorType = type === 'liquid' ? 'liquid' : 'sensor';
  var els = _getSensorLinkElements();
  var input = els.input;
  var btn = els.btn;
  var btnTxt = els.btnTxt;
  var spinner = els.spinner;
  var errEl = els.errEl;
  var serial = (input ? input.value : '').trim();

  if (!serial) {
    if (errEl) { errEl.textContent = 'Lütfen bir seri numarası girin.'; errEl.classList.remove('hidden'); }
    return;
  }
  if (errEl) errEl.classList.add('hidden');
  if (btn) btn.disabled = true;
  if (spinner) spinner.classList.remove('hidden');
  if (btnTxt) btnTxt.textContent = 'Doğrulanıyor…';

  fetch('/api/sensor-link', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sensor_serial: serial })
  })
    .then(function(r) { return r.json().then(function(d) { return { ok: r.ok, data: d }; }); })
    .then(function(res) {
      if (!res.ok) {
        var msg = (res.data && res.data.error === 'sensor_not_found')
          ? 'Bu seri numaralı sensör bulunamadı. Numarayı kontrol edin.'
          : 'Bir hata oluştu, lütfen tekrar deneyin.';
        if (errEl) { errEl.textContent = msg; errEl.classList.remove('hidden'); }
        if (btn) btn.disabled = false;
        if (spinner) spinner.classList.add('hidden');
        if (btnTxt) btnTxt.textContent = 'Bağla';
        return;
      }

      if (sensorType === 'liquid') {
        _unhideEnvironmentItem('liquid', serial);
        _activeLiquidSensorSerial = serial;
        _storeActiveSelection('liquid', serial);
      } else {
        _unhideEnvironmentItem('air', serial);
        _linkedSensorSerial = serial;
        _activeAirSensorSerial = serial;
        _storeActiveSelection('air', serial);
      }

      _rememberSensor(serial, '', sensorType === 'liquid' ? 'liquid' : 'air');
      if (input) input.value = '';
      loadSensorList();
      _showDataView();
      _startPolling();
      closeSensorAddModal();
      closeEnvironmentPicker();
    })
    .catch(function() {
      if (errEl) { errEl.textContent = 'Sunucuya ulaşılamadı.'; errEl.classList.remove('hidden'); }
      if (btn) btn.disabled = false;
      if (spinner) spinner.classList.add('hidden');
      if (btnTxt) btnTxt.textContent = 'Bağla';
    });
}

function linkCamera() {
  var input = document.getElementById('cameraModalSerialInput');
  var btn = document.getElementById('cameraModalLinkBtn') || document.getElementById('sensorModalLinkBtn');
  var btnTxt = document.getElementById('cameraModalLinkBtnText');
  var spinner = document.getElementById('cameraModalLinkSpinner');
  var errEl = document.getElementById('cameraModalLinkError');
  var serial = (input ? input.value : '').trim();

  if (!serial) {
    if (errEl) { errEl.textContent = 'Lütfen bir kamera seri numarası girin.'; errEl.classList.remove('hidden'); }
    return;
  }
  if (errEl) errEl.classList.add('hidden');
  if (btn) btn.disabled = true;
  if (spinner) spinner.classList.remove('hidden');
  if (btnTxt) btnTxt.textContent = 'Bağlanıyor...';

  fetch('/api/camera-link', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ camera_serial: serial })
  })
    .then(function(r) { return r.json().then(function(d) { return { ok: r.ok, data: d }; }); })
    .then(function(res) {
      if (!res.ok) {
        var msg = 'Kamera bağlanamadı, lütfen tekrar deneyin.';
        if (res.data && res.data.error === 'camera_not_found') {
          msg = 'Bu seri numaralı kamera bulunamadı. Önce kameradan görüntü geldiğini ve seri numarasını kontrol edin.';
        } else if (res.data && res.data.error === 'management_camera_route_missing') {
          msg = 'Yönetim sunucusunda kamera API güncellemesi aktif değil. Systrack Management backend yeniden build/deploy edilmeli.';
        }
        if (errEl) { errEl.textContent = msg; errEl.classList.remove('hidden'); }
        return;
      }

      _unhideEnvironmentItem('camera', serial.toUpperCase());
      _activeCameraSerial = serial.toUpperCase();
      _storeActiveSelection('camera', _activeCameraSerial);
      _rememberCamera(_activeCameraSerial, '');
      if (input) input.value = '';
      closeCameraAddModal();
      closeEnvironmentPicker();
      loadCameraPreviewData(true);
    })
    .catch(function() {
      if (errEl) { errEl.textContent = 'Sunucuya ulaşılamadı.'; errEl.classList.remove('hidden'); }
    })
    .finally(function() {
      if (btn) btn.disabled = false;
      if (spinner) spinner.classList.add('hidden');
      if (btnTxt) btnTxt.textContent = 'Bağla';
    });
}

function loadCameraPreviewData(force) {
  fetch('/api/cameras')
    .then(function(r) { return r.ok ? r.json() : { cameras: [] }; })
    .then(function(data) {
      var hidden = _getHiddenTypedSet('camera');
      _allCameras = _mergeSensorLists(Array.isArray(data.cameras) ? data.cameras : [], _getStoredTypedList('camera')).filter(function(camera) {
        var serial = String(camera && camera.serial || '').trim();
        return serial && !hidden.has(serial);
      });
      if (!_activeCameraSerial || force || !_allCameras.some(function(c) { return c.serial === _activeCameraSerial; })) {
        _activeCameraSerial = _allCameras[0] ? _allCameras[0].serial : '';
      }
      _renderEnvironmentDropdowns();
      if (!_activeCameraSerial) {
        renderCameraPreview([], null);
        return;
      }
      return loadCameraImages(_activeCameraSerial);
    })
    .catch(function() {
      renderCameraPreview([], null, 'Kamera bilgileri alınamadı.');
    });

  if (!_cameraPollTimer) {
    _cameraPollTimer = setInterval(function() {
      if (_activeCameraSerial) loadCameraImages(_activeCameraSerial);
    }, 15000);
  }
}

/* -----------------------------
   Export
----------------------------- */
if (typeof window !== 'undefined') {
  window.initSensorsPage = initSensorsPage;
  window.applyEnvironmentSelections = applyEnvironmentSelections;
  window.toggleEnvironmentPicker = toggleEnvironmentPicker;
  window.closeEnvironmentPicker = closeEnvironmentPicker;
  window.selectEnvironmentItem = selectEnvironmentItem;
  window.removeEnvironmentItem = removeEnvironmentItem;
  window.openSensorAddModal = openSensorAddModal;
  window.closeSensorAddModal = closeSensorAddModal;
  window.openCameraAddModal = openCameraAddModal;
  window.closeCameraAddModal = closeCameraAddModal;
  window.openLiquidHistoryModal = openLiquidHistoryModal;
  window.closeLiquidHistoryModal = closeLiquidHistoryModal;
  window.openAddModal = openAddModal;
  window.closeAddModal = closeAddModal;
  window.switchAddModalTab = switchAddModalTab;
  window.submitAddModal = submitAddModal;
  window.linkCamera = linkCamera;
  window.moveCameraImage = moveCameraImage;
  window.moveCameraLightbox = moveCameraLightbox;
  window.closeCameraLightbox = closeCameraLightbox;
  window.openCameraDetailModal = openCameraDetailModal;
  window.closeCameraDetailModal = closeCameraDetailModal;
  window.showCameraDateFolders = showCameraDateFolders;
  window.showCameraImagesForDate = showCameraImagesForDate;
  window.setGaugeValue   = setGaugeValue;
  window.setThermoValue  = setThermoValue;
}
