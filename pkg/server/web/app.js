let ws;
let currentStep = 1;
let currentImageBase64 = null;
let viewMode = 'vec'; // 'orig', 'seg', 'vec'
let lastSVG = "";
let lastSegPNG = "";

// Pipeline Configuration State
const state = {
  colors: 6,
  min_pixels: 4,
  reject_aa: true,
  smoothness: 0.50,
  detect_corners: true,
  tolerance: 0.75,
  anti_seam: true,
  transparent_bg: false
};

// Error feedback banner helpers
function showError(msg) {
  hideProgress();
  console.error("[Vectorizador]", msg);
  const banner = document.getElementById('error-banner');
  const text = document.getElementById('error-message');
  if (banner && text) {
    text.textContent = msg;
    banner.style.display = 'flex';
  } else {
    alert(msg);
  }
}

function hideError() {
  const banner = document.getElementById('error-banner');
  if (banner) {
    banner.style.display = 'none';
  }
}

// Initialize WebSocket connection for slider interaction
function initWebSocket() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/ws`;

  ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    document.getElementById('connection-status').innerHTML = '<span style="color: #10b981;">● Conectado (WebSocket)</span>';
  };

  ws.onclose = () => {
    document.getElementById('connection-status').innerHTML = '<span style="color: #ef4444;">● Desconectado - reconectando...</span>';
    setTimeout(initWebSocket, 2000);
  };

  ws.onerror = (err) => {
    console.error("Error WebSocket:", err);
  };

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      handleServerMessage(msg);
    } catch (e) {
      console.error("Error al procesar mensaje WebSocket:", e);
    }
  };
}

function handleServerMessage(msg) {
  switch (msg.type) {
    case 'progress':
      showProgress(msg.stage, msg.percent);
      break;
    case 'palette':
      renderPaletteSwatches(msg.colors);
      break;
    case 'segmentation':
      lastSegPNG = msg.preview;
      if (viewMode === 'seg') updateWorkspaceView();
      break;
    case 'svg':
      lastSVG = msg.svg;
      updateWorkspaceView();
      hideProgress();
      break;
    case 'error':
      showError('Error en el motor: ' + msg.error);
      break;
  }
}

function showProgress(stage, percent) {
  const box = document.getElementById('progress-box');
  const label = document.getElementById('progress-label');
  const bar = document.getElementById('progress-bar');
  if (box && label && bar) {
    box.classList.add('active');
    label.textContent = stage;
    bar.style.width = percent + '%';
  }
}

function hideProgress() {
  const box = document.getElementById('progress-box');
  if (box) {
    setTimeout(() => box.classList.remove('active'), 250);
  }
}

// Stepper Navigation
function goToStep(step) {
  currentStep = step;

  document.querySelectorAll('.step-item').forEach(el => {
    const s = parseInt(el.getAttribute('data-step'));
    el.classList.remove('active');
    if (s === step) el.classList.add('active');
    if (s < step) el.classList.add('completed');
  });

  document.querySelectorAll('.stage-panel').forEach(el => {
    el.classList.remove('active');
  });
  const currentPanel = document.getElementById(`stage-panel-${step}`);
  if (currentPanel) {
    currentPanel.classList.add('active');
  }

  // Trigger preview switch matching step
  if (step === 2) {
    setViewMode('seg');
  } else if (step >= 3) {
    setViewMode('vec');
  }

  triggerPipeline();
}

// View Mode Switching ('orig', 'seg', 'vec')
function setViewMode(mode) {
  viewMode = mode;
  ['orig', 'seg', 'vec'].forEach(m => {
    const btn = document.getElementById(`view-mode-${m}`);
    if (btn) {
      if (m === mode) {
        btn.style.background = 'var(--accent)';
        btn.style.color = 'white';
      } else {
        btn.style.background = 'var(--bg-card)';
        btn.style.color = 'var(--text-main)';
      }
    }
  });
  updateWorkspaceView();
}

function updateWorkspaceView() {
  const placeholder = document.getElementById('placeholder-text');
  const imgEl = document.getElementById('img-preview');
  const svgEl = document.getElementById('svg-preview');

  if (!currentImageBase64) {
    if (placeholder) placeholder.style.display = 'block';
    if (imgEl) imgEl.style.display = 'none';
    if (svgEl) svgEl.style.display = 'none';
    return;
  }

  if (placeholder) placeholder.style.display = 'none';

  if (viewMode === 'orig') {
    imgEl.src = currentImageBase64;
    imgEl.style.display = 'block';
    svgEl.style.display = 'none';
  } else if (viewMode === 'seg') {
    if (lastSegPNG) {
      imgEl.src = lastSegPNG;
      imgEl.style.display = 'block';
      svgEl.style.display = 'none';
    } else {
      imgEl.src = currentImageBase64;
      imgEl.style.display = 'block';
      svgEl.style.display = 'none';
    }
  } else if (viewMode === 'vec') {
    if (lastSVG) {
      svgEl.innerHTML = lastSVG;
      svgEl.style.display = 'block';
      imgEl.style.display = 'none';
    } else {
      imgEl.src = currentImageBase64;
      imgEl.style.display = 'block';
      svgEl.style.display = 'none';
    }
  }
}

function renderPaletteSwatches(colors) {
  const container = document.getElementById('palette-swatches');
  if (!container) return;
  container.innerHTML = '';
  if (!colors || colors.length === 0) return;

  colors.forEach(hex => {
    const swatch = document.createElement('div');
    swatch.className = 'swatch';
    swatch.style.backgroundColor = hex;
    swatch.title = hex;
    container.appendChild(swatch);
  });
}

// Pipeline Dispatch via WebSocket for live parameter changes
let debounceTimer;
function triggerPipeline() {
  if (!currentImageBase64) return;
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    showError("La conexión con el motor no está lista. Por favor espera la reconexión...");
    return;
  }

  clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    hideError();
    showProgress('Actualizando vectorización...', 15);
    ws.send(JSON.stringify({
      action: 'reprocess',
      step: currentStep,
      config: state
    }));
  }, 120);
}

// Upload Handling via HTTP multipart POST (handles multi-megabyte files reliably)
function handleFileSelect(file) {
  if (!file) return;

  hideError();
  const sizeMB = (file.size / (1024 * 1024)).toFixed(2);
  showProgress(`Cargando archivo '${file.name}' (${sizeMB} MB)...`, 15);

  // Set local object URL immediately so original view displays
  try {
    currentImageBase64 = URL.createObjectURL(file);
    updateWorkspaceView();
  } catch (e) {
    console.warn("Could not create object URL:", e);
  }

  const formData = new FormData();
  formData.append('file', file);
  formData.append('config', JSON.stringify(state));

  showProgress(`Enviando '${file.name}' al motor y vectorizando...`, 25);

  fetch('/api/upload', {
    method: 'POST',
    body: formData
  })
  .then(async (response) => {
    let data;
    try {
      data = await response.json();
    } catch (e) {
      throw new Error(`El servidor respondió con código ${response.status} (sin datos JSON válidos)`);
    }

    if (!response.ok || !data || !data.success) {
      const err = (data && data.error) ? data.error : `Error en la subida (código HTTP ${response.status})`;
      throw new Error(err);
    }
    return data;
  })
  .then(data => {
    lastSegPNG = data.segmentation;
    lastSVG = data.svg;
    renderPaletteSwatches(data.palette);
    updateWorkspaceView();
    hideProgress();
  })
  .catch(err => {
    console.error("Error al subir archivo:", err);
    showError("Error al procesar imagen: " + err.message);
  });
}

// Load sample directly from server testdata
function loadSample(name) {
  hideError();
  showProgress(`Cargando muestra '${name}' en el servidor...`, 15);

  currentImageBase64 = `/api/samples/${encodeURIComponent(name)}`;
  updateWorkspaceView();

  fetch(`/api/samples/load?name=${encodeURIComponent(name)}`, {
    method: 'POST'
  })
  .then(async (response) => {
    let data;
    try {
      data = await response.json();
    } catch (e) {
      throw new Error(`Error HTTP ${response.status}`);
    }

    if (!response.ok || !data || !data.success) {
      const err = (data && data.error) ? data.error : `Error al cargar muestra (HTTP ${response.status})`;
      throw new Error(err);
    }
    return data;
  })
  .then(data => {
    lastSegPNG = data.segmentation;
    lastSVG = data.svg;
    renderPaletteSwatches(data.palette);
    updateWorkspaceView();
    hideProgress();
  })
  .catch(err => {
    console.error("Error al cargar muestra:", err);
    showError("No se pudo cargar la muestra: " + err.message);
  });
}

// Populate sample buttons from /api/samples
function populateSampleList() {
  fetch('/api/samples')
    .then(r => r.json())
    .then(list => {
      const container = document.getElementById('sample-buttons-container');
      if (!container || !list) return;
      container.innerHTML = '';
      list.forEach(name => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'btn-sample';
        btn.textContent = name;
        btn.style.margin = '2px';
        btn.onclick = () => loadSample(name);
        container.appendChild(btn);
      });
    })
    .catch(err => console.log('No se pudieron listar muestras:', err));
}

// Export actions
function downloadSVG() {
  if (!lastSVG) {
    showError('No hay ningún gráfico SVG generado aún para descargar.');
    return;
  }
  const blob = new Blob([lastSVG], { type: 'image/svg+xml;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'vectorizado.svg';
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function copySVGToClipboard() {
  if (!lastSVG) {
    showError('No hay ningún gráfico SVG generado aún para copiar.');
    return;
  }
  navigator.clipboard.writeText(lastSVG).then(() => {
    alert('Código SVG copiado al portapapeles!');
  }).catch(err => {
    showError('No se pudo copiar al portapapeles: ' + err);
  });
}

// DOM Setup
window.addEventListener('DOMContentLoaded', () => {
  initWebSocket();
  populateSampleList();

  // Dropzone and File Input
  const dropzone = document.getElementById('dropzone');
  const fileInput = document.getElementById('file-input');

  if (dropzone && fileInput) {
    dropzone.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      fileInput.click();
    });

    fileInput.addEventListener('change', (e) => {
      if (e.target.files && e.target.files.length > 0) {
        handleFileSelect(e.target.files[0]);
      }
    });

    dropzone.addEventListener('dragover', (e) => {
      e.preventDefault();
      dropzone.classList.add('dragover');
    });

    dropzone.addEventListener('dragleave', () => {
      dropzone.classList.remove('dragover');
    });

    dropzone.addEventListener('drop', (e) => {
      e.preventDefault();
      dropzone.classList.remove('dragover');
      if (e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length > 0) {
        handleFileSelect(e.dataTransfer.files[0]);
      }
    });
  }

  // Slider bindings
  const bindSlider = (id, valId, key, transform) => {
    const el = document.getElementById(id);
    const valEl = document.getElementById(valId);
    if (!el || !valEl) return;
    el.oninput = () => {
      const v = transform ? transform(el.value) : el.value;
      valEl.textContent = typeof v === 'number' && !Number.isInteger(v) ? v.toFixed(2) : v;
      state[key] = v;
      triggerPipeline();
    };
  };

  bindSlider('color-count-slider', 'color-count-val', 'colors', v => parseInt(v));
  bindSlider('min-pixels-slider', 'min-pixels-val', 'min_pixels', v => parseInt(v));
  bindSlider('smoothness-slider', 'smoothness-val', 'smoothness', v => parseFloat(v) / 100.0);
  bindSlider('tolerance-slider', 'tolerance-val', 'tolerance', v => parseFloat(v) / 100.0);

  // Checkbox bindings
  const bindCheckbox = (id, key) => {
    const el = document.getElementById(id);
    if (!el) return;
    el.onchange = () => {
      state[key] = el.checked;
      triggerPipeline();
    };
  };

  bindCheckbox('reject-aa-checkbox', 'reject_aa');
  bindCheckbox('detect-corners-checkbox', 'detect_corners');
  bindCheckbox('anti-seam-checkbox', 'anti_seam');
  bindCheckbox('transparent-bg-checkbox', 'transparent_bg');
});