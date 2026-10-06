(function () {
  'use strict';

  var esc = function (s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  };

  // Mobile navigation
  var navToggle = document.querySelector('[data-nav-toggle]');
  if (navToggle) {
    navToggle.addEventListener('click', function () {
      document.querySelector('[data-nav]').classList.toggle('is-open');
    });
  }

  // Scrolling hero photos
  document.querySelectorAll('[data-slider]').forEach(function (slider) {
    var track = slider.querySelector('.hero__track');
    var count = track.children.length;
    if (count < 2) return;
    var dotsWrap = slider.querySelector('[data-slider-dots]');
    var reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    var index = 0, timer = null, dots = [];

    function go(i) {
      index = (i + count) % count;
      track.style.transform = 'translateX(' + (-100 * index) + '%)';
      dots.forEach(function (d, j) { d.classList.toggle('is-active', j === index); });
    }
    function stop() { clearInterval(timer); timer = null; }
    function start() { stop(); if (!reduceMotion) timer = setInterval(function () { go(index + 1); }, 5500); }

    for (var k = 0; k < count; k++) {
      var b = document.createElement('button');
      b.type = 'button';
      b.setAttribute('aria-label', 'Show photo ' + (k + 1));
      b.addEventListener('click', go.bind(null, k));
      dotsWrap.appendChild(b);
      dots.push(b);
    }
    slider.querySelector('[data-slider-prev]').addEventListener('click', function () { go(index - 1); start(); });
    slider.querySelector('[data-slider-next]').addEventListener('click', function () { go(index + 1); start(); });
    slider.addEventListener('mouseenter', stop);
    slider.addEventListener('mouseleave', start);

    var touchX = null;
    slider.addEventListener('touchstart', function (e) { touchX = e.touches[0].clientX; stop(); }, { passive: true });
    slider.addEventListener('touchend', function (e) {
      if (touchX === null) return;
      var dx = e.changedTouches[0].clientX - touchX;
      if (Math.abs(dx) > 40) go(index + (dx < 0 ? 1 : -1));
      touchX = null; start();
    });
    go(0); start();
  });

  // Valuation / get in touch switch
  var form = document.querySelector('[data-enquiry-form]');
  function setKind(kind) {
    if (!form) return;
    var isValuation = kind === 'valuation';
    form.elements.namedItem('kind').value = isValuation ? 'valuation' : 'contact';
    document.querySelectorAll('[data-kind-option]').forEach(function (b) {
      var on = b.getAttribute('data-kind-option') === (isValuation ? 'valuation' : 'contact');
      b.classList.toggle('is-active', on);
      b.setAttribute('aria-pressed', on ? 'true' : 'false');
    });
    form.elements.namedItem('address').required = isValuation;
    form.querySelector('[data-addr-req]').hidden = !isValuation;
    form.querySelector('[data-addr-label]').textContent = isValuation ? 'Address of property to value' : 'Your address';
    form.querySelector('[data-submit]').textContent = isValuation ? 'Request my valuation' : 'Send message';
  }
  document.querySelectorAll('[data-kind-option]').forEach(function (b) {
    b.addEventListener('click', function () { setKind(b.getAttribute('data-kind-option')); });
  });
  document.querySelectorAll('[data-goto-kind]').forEach(function (a) {
    a.addEventListener('click', function (e) {
      var target = document.getElementById('contact');
      if (!target || !form) return;
      e.preventDefault();
      setKind(a.getAttribute('data-goto-kind'));
      var nav = document.querySelector('[data-nav]');
      if (nav) nav.classList.remove('is-open');
      target.scrollIntoView({ behavior: 'smooth' });
      history.replaceState(null, '', '#contact');
    });
  });

  // Captcha refresh
  document.querySelectorAll('[data-captcha-reload]').forEach(function (b) {
    b.addEventListener('click', function () {
      var img = document.querySelector('[data-captcha-img]');
      var url = new URL(img.getAttribute('src'), location.href);
      url.searchParams.set('reload', Date.now());
      img.src = url.pathname + url.search;
    });
  });

  // Office map
  var mapEl = document.getElementById('office-map');
  if (mapEl && window.L) {
    var lat = parseFloat(mapEl.dataset.lat), lng = parseFloat(mapEl.dataset.lng);
    var zoom = parseInt(mapEl.dataset.zoom, 10) || 14;
    var map = L.map(mapEl, { scrollWheelZoom: false }).setView([lat, lng], zoom);
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
      maxZoom: 19,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
    }).addTo(map);
    var officeIcon = L.divIcon({ className: '', html: '<span class="office-pin"></span>', iconSize: [30, 30], iconAnchor: [15, 34], popupAnchor: [0, -30] });
    var office = L.marker([lat, lng], { icon: officeIcon, zIndexOffset: 1000 }).addTo(map)
      .bindPopup('<strong>' + esc(mapEl.dataset.name) + '</strong><br>' + esc(mapEl.dataset.address));
    var points = [office.getLatLng()];
    var markers = [];
    try { markers = JSON.parse(mapEl.dataset.markers || '[]'); } catch (e) { markers = []; }
    markers.forEach(function (m) {
      var mk = L.marker([m.lat, m.lng]).addTo(map)
        .bindPopup('<a href="' + esc(m.url) + '"><strong>' + esc(m.price) + '</strong><br>' + esc(m.address) + '</a>');
      points.push(mk.getLatLng());
    });
    map.on('click', function () { map.scrollWheelZoom.enable(); });
    mapEl.addEventListener('mouseleave', function () { map.scrollWheelZoom.disable(); });
    var officeBtn = document.querySelector('[data-map-office]');
    if (officeBtn) officeBtn.addEventListener('click', function () { map.flyTo([lat, lng], Math.min(zoom + 2, 18)); office.openPopup(); });
    var allBtn = document.querySelector('[data-map-all]');
    if (allBtn) allBtn.addEventListener('click', function () { map.flyToBounds(L.latLngBounds(points).pad(0.15)); });
  }

  // Property gallery
  document.querySelectorAll('[data-gallery]').forEach(function (g) {
    var main = g.querySelector('[data-gallery-main]');
    var thumbs = g.querySelectorAll('[data-gallery-thumb]');
    thumbs.forEach(function (t) {
      t.addEventListener('click', function () {
        main.src = t.getAttribute('data-src');
        thumbs.forEach(function (x) { x.classList.toggle('is-active', x === t); });
      });
    });
  });

  // Confirmation for destructive admin actions
  document.querySelectorAll('form[data-confirm]').forEach(function (f) {
    f.addEventListener('submit', function (e) {
      if (!window.confirm(f.getAttribute('data-confirm'))) e.preventDefault();
    });
  });
})();
