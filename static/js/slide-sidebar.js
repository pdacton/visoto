// static/js/slide-sidebar.js
// Shared slide-in/out mechanics for the two sidebars: the nav on the left
// (sidebar.js) and the assistant on the right (right-sidebar.js).
//
// Desktop: the sidebar hides behind a negative margin on its own side, and
// .page-wrapper's margin on that side follows it, so the content is squeezed
// rather than covered. The open/closed state is remembered by the caller.
//
// Mobile (below Tabler's lg breakpoint): opening shows the sidebar's
// .navbar-collapse menu; with `overlay` set, the sidebar also slides in over
// the page behind a backdrop. A mobile open is a gesture for this page only,
// so it is never written back to the remembered desktop state.

(function () {
  const MOBILE_BREAKPOINT = 992;
  const EASE = '0.3s cubic-bezier(.4,0,.2,1)';

  function isMobile() {
    return window.innerWidth < MOBILE_BREAKPOINT;
  }

  // opts:
  //   el         the <aside>
  //   side       'left' | 'right'
  //   menu       its .navbar-collapse (shown on mobile open)
  //   toggles    elements whose click toggles the sidebar
  //   width()    desktop width in px, read on every open
  //   sizeEl     also pin el's width inline (the resizable right sidebar)
  //   readOpen() / writeOpen(open)   the remembered desktop state
  //   overlay    { className, backdropClass } for the mobile overlay, or omitted
  window.VisotoSlideSidebar = function (opts) {
    const el = opts.el;
    const pageWrapper = document.querySelector('.page-wrapper');
    const margin = opts.side === 'left' ? 'marginLeft' : 'marginRight';
    const elTransition = 'margin-' + opts.side + ' ' + EASE + ', transform ' + EASE;
    const overlay = opts.overlay;
    let open = false;
    let backdropRemoval = null;

    // Off while the state is applied on load or across a breakpoint, so the
    // sidebar jumps to where it belongs instead of sliding there.
    // .page-wrapper's own transition lives in tabler_overrides.css.
    function setTransitions(on) {
      el.style.transition = on ? elTransition : 'none';
      pageWrapper.style.transition = on ? '' : 'none';
    }

    function applyDesktop() {
      const width = opts.width();
      el.style[margin] = open ? '0' : '-' + width + 'px';
      if (opts.sizeEl) el.style.width = width + 'px';
      pageWrapper.style[margin] = open ? width + 'px' : '0';
    }

    function applyMobile() {
      if (opts.menu) opts.menu.classList.toggle('show', open);
      if (!overlay) return;
      el.classList.toggle('show', open);
      if (open) addBackdrop(); else removeBackdrop();
    }

    function addBackdrop() {
      clearTimeout(backdropRemoval);
      let backdrop = document.querySelector('.' + overlay.backdropClass);
      if (!backdrop) {
        backdrop = document.createElement('div');
        backdrop.className = overlay.backdropClass;
        backdrop.addEventListener('click', function () { setOpen(false); });
        document.body.appendChild(backdrop);
      }
      setTimeout(function () { backdrop.classList.add('show'); }, 10);
    }

    function removeBackdrop() {
      const backdrop = document.querySelector('.' + overlay.backdropClass);
      if (!backdrop) return;
      backdrop.classList.remove('show');
      backdropRemoval = setTimeout(function () { backdrop.remove(); }, 300);
    }

    function setOpen(next) {
      open = next;
      if (isMobile()) {
        applyMobile();
      } else {
        applyDesktop();
        opts.writeOpen(open);
      }
    }

    // Lay the sidebar out for the current breakpoint. Mobile always starts
    // closed; desktop restores the remembered state.
    function initialize() {
      setTransitions(false);
      if (isMobile()) {
        el.style[margin] = '';
        el.style.width = '';
        pageWrapper.style[margin] = '';
        open = false;
        // The overlay class stays on for as long as the page is narrow, so that
        // opening is only the `show` toggle, which is what slides it in.
        if (overlay) el.classList.add(overlay.className);
        applyMobile();
      } else {
        if (overlay) {
          el.classList.remove(overlay.className, 'show');
          removeBackdrop();
        }
        if (opts.menu) opts.menu.classList.remove('show');
        open = opts.readOpen();
        applyDesktop();
      }
      setTimeout(function () { setTransitions(true); }, 100);
    }

    initialize();

    (opts.toggles || []).forEach(function (toggle) {
      if (!toggle) return;
      toggle.addEventListener('click', function (e) {
        e.preventDefault();
        setOpen(!open);
      });
    });

    let wasMobile = isMobile();
    window.addEventListener('resize', function () {
      if (wasMobile === isMobile()) return;
      wasMobile = isMobile();
      initialize();
    });

    return {
      isOpen: function () { return open; },
      isMobile: isMobile,
      close: function () { setOpen(false); },
      setTransitions: setTransitions,
      // Re-apply the desktop layout after the caller's width() changed.
      relayout: applyDesktop,
    };
  };
})();
