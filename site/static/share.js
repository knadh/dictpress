async function screenshotDOM(el, opts = {}) {
    const {
        scale = 2,
        type = 'image/png',
        quality = 0.92,
        background = '#fff',
        // if you want to *allow* splitting (and risk the bug), set to false
        preventCapsuleSplit = true
    } = opts;

    if (!(el instanceof Element)) throw new TypeError('Expected a DOM Element');

    if (document.fonts && document.fonts.status !== 'loaded') {
        try { await document.fonts.ready; } catch { }
    }

    const { width, height } = (() => {
        const r = el.getBoundingClientRect();
        return { width: Math.ceil(r.width), height: Math.ceil(r.height) };
    })();
    if (!width || !height) throw new Error('Element has zero width/height.');

    const clone = el.cloneNode(true);
    await inlineEverything(el, clone);

    if (background !== 'transparent') clone.style.background = background;
    if (!clone.getAttribute('xmlns')) clone.setAttribute('xmlns', 'http://www.w3.org/1999/xhtml');

    const xhtml = new XMLSerializer().serializeToString(clone);
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}">
    <foreignObject x="0" y="0" width="100%" height="100%">${xhtml}</foreignObject>
  </svg>`;

    const svgBlob = new Blob([svg], { type: 'image/svg+xml;charset=utf-8' });
    const url = URL.createObjectURL(svgBlob);

    try {
        const img = await loadImage(url);
        const canvas = document.createElement('canvas');
        canvas.width = Math.ceil(width * scale);
        canvas.height = Math.ceil(height * scale);
        const ctx = canvas.getContext('2d');
        if (background !== 'transparent' || type !== 'image/png') {
            ctx.fillStyle = background === 'transparent' ? '#0000' : background;
            ctx.fillRect(0, 0, canvas.width, canvas.height);
        }
        ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
        const blob = await new Promise(res => canvas.toBlob(res, type, quality));
        if (!blob) throw new Error('Canvas.toBlob returned null.');
        return blob;
    } finally {
        URL.revokeObjectURL(url);
    }

    function loadImage(src) {
        return new Promise((resolve, reject) => {
            const img = new Image();
            img.crossOrigin = 'anonymous';
            img.decoding = 'async';
            img.onload = () => resolve(img);
            img.onerror = reject;
            img.src = src;
        });
    }

    async function inlineEverything(srcNode, dstNode) {
        copyComputedStyle(srcNode, dstNode);

        if (srcNode instanceof HTMLTextAreaElement) {
            dstNode.textContent = srcNode.value;
        } else if (srcNode instanceof HTMLInputElement) {
            dstNode.setAttribute('value', srcNode.value);
            if ((srcNode.type === 'checkbox' || srcNode.type === 'radio') && srcNode.checked) {
                dstNode.setAttribute('checked', '');
            }
        } else if (srcNode instanceof HTMLSelectElement) {
            const sel = Array.from(srcNode.options).filter(o => o.selected).map(o => o.value);
            Array.from(dstNode.options).forEach(o => (o.selected = sel.includes(o.value)));
        }

        if (srcNode instanceof HTMLCanvasElement) {
            try {
                const dataURL = srcNode.toDataURL();
                const img = document.createElement('img');
                img.src = dataURL;
                copyBoxSizing(dstNode, img);
                dstNode.replaceWith(img);
                dstNode = img;
            } catch { }
        }

        materializePseudo(srcNode, dstNode, '::before');
        materializePseudo(srcNode, dstNode, '::after');

        const sKids = srcNode.childNodes;
        const dKids = dstNode.childNodes;
        for (let i = 0; i < sKids.length; i++) {
            const s = sKids[i], d = dKids[i];
            if (s && d && s.nodeType === 1 && d.nodeType === 1) await inlineEverything(s, d);
        }

        function copyComputedStyle(src, dst) {
            const cs = window.getComputedStyle(src);
            let cssText = '';
            for (const prop of cs) cssText += `${prop}:${cs.getPropertyValue(prop)};`;
            dst.setAttribute('style', (dst.getAttribute('style') || '') + cssText);

            // Always keep transform origin consistent
            if (cs.transformOrigin) dst.style.transformOrigin = cs.transformOrigin;

            // Keep each line painting its own decoration (helps some engines)
            dst.style.setProperty('box-decoration-break', 'clone', 'important');
            dst.style.setProperty('-webkit-box-decoration-break', 'clone', 'important');

            // --- CAPSULE FIX: prevent decorated inline from splitting across lines ---
            if (preventCapsuleSplit) {
                const hasBg = (cs.backgroundImage && cs.backgroundImage !== 'none') ||
                    (cs.backgroundColor && cs.backgroundColor !== 'rgba(0, 0, 0, 0)' && cs.backgroundColor !== 'transparent');
                const hasRadius = ['borderTopLeftRadius', 'borderTopRightRadius', 'borderBottomRightRadius', 'borderBottomLeftRadius']
                    .some(k => parseFloat(cs[k]) > 0);
                const hasPad = ['paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft']
                    .some(k => parseFloat(cs[k]) > 0);
                const isInlineLevel = cs.display.startsWith('inline');

                if (isInlineLevel && (hasBg || hasRadius || hasPad)) {
                    // Make it behave like a chip for capture: no internal wrapping
                    dst.style.setProperty('display', 'inline-block', 'important');
                    dst.style.setProperty('white-space', 'nowrap', 'important');
                    // In case original allowed breaking long words, emulate by clipping
                    if (cs.overflowWrap === 'break-word' || cs.wordBreak === 'break-all') {
                        dst.style.setProperty('max-width', cs.maxWidth && cs.maxWidth !== 'none' ? cs.maxWidth : '100%', 'important');
                        dst.style.setProperty('overflow', 'hidden', 'important');
                        dst.style.setProperty('text-overflow', 'ellipsis', 'important');
                    }
                }
            }
        }

        function copyBoxSizing(src, dst) {
            const cs = window.getComputedStyle(src);
            dst.style.width = cs.width;
            dst.style.height = cs.height;
            dst.style.display = cs.display;
            dst.style.objectFit = 'contain';
        }

        function materializePseudo(src, dst, which) {
            const ps = window.getComputedStyle(src, which);
            if (!ps || ps.content === '' || ps.content === 'none') return;
            const span = document.createElement('span');
            let cssText = '';
            for (const prop of ps) cssText += `${prop}:${ps.getPropertyValue(prop)};`;
            span.setAttribute('style', cssText);

            const content = ps.content;
            const quoted = /^(['"]).*\1$/.test(content);
            if (quoted) span.textContent = content.slice(1, -1).replace(/\\n/g, '\n');
            else if (content.startsWith('attr(')) {
                const attrName = content.slice(5, -1).trim();
                span.textContent = src.getAttribute(attrName) || '';
            } else span.textContent = '';

            if (which === '::before') dst.insertBefore(span, dst.firstChild);
            else dst.appendChild(span);
        }
    }
}

async function shareDOM(target, title, text, filename) {
    try {
        // Make a crisp image (PNG keeps transparency; use JPEG for smaller files)
        const blob = await screenshotDOM(target, {
            scale: Math.max(2, window.devicePixelRatio || 2),
            type: 'image/png',
            background: 'white'
        });

        const file = new File([blob], filename || 'share.png', {
            type: blob.type,
            lastModified: Date.now()
        });

        // Web Share API with files (Android Chrome, iOS/iPadOS Safari 16+).
        if (navigator.canShare && navigator.canShare({ files: [file] })) {
            await navigator.share({ title, text, files: [file] });
            return;
        }

        // Or share a blob URL (some browsers only accept url/text).
        if (navigator.share) {
            const url = URL.createObjectURL(blob);
            try {
                await navigator.share({ title, url });
            } finally {
                URL.revokeObjectURL(url);
            }
            return;
        }

        // Or a copy image to clipboard (desktop Chrome/Edge, some Android)/
        if (navigator.clipboard && window.ClipboardItem) {
            await navigator.clipboard.write([new ClipboardItem({ [blob.type]: blob })]);
            alert('Screenshot copied to clipboard');
            return;
        }

        // Nothing worked, prompt a download.
        const dlUrl = URL.createObjectURL(blob);
        const a = Object.assign(document.createElement('a'), {
            href: dlUrl,
            download: 'share.png'
        });
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(dlUrl);
    } catch (err) {
        console.error(err);
        alert(`share failed: ${err?.message || err}`);
    } finally { }
}
