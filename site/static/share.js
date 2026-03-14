// Canvas-based entry card screenshot and share.

function renderEntryCard(entryEl) {
  const cs = getComputedStyle(document.documentElement);
  const colors = {
    bg: '#fff',
    primary: cs.getPropertyValue('--primary').trim() || '#111',
    light: cs.getPropertyValue('--light').trim() || '#666',
    lighter: cs.getPropertyValue('--lighter').trim() || '#aaa',
    border: '#e6e6e6',
  };
  const fontFamily = getComputedStyle(document.body).fontFamily;
  const font = (size, bold) => `${bold ? 'bold ' : ''}${size}px ${fontFamily}`;

  // Extract data from entry DOM.
  const headword = entryEl.dataset.head || '';
  const pronun = entryEl.querySelector('.pronun')?.textContent?.trim() || '';

  // Collect definition groups: [{type, defs}]
  const groups = [];
  entryEl.querySelectorAll('ol.defs').forEach((ol) => {
    let typeLabel = '';
    const defs = [];
    ol.querySelectorAll(':scope > li').forEach((li) => {
      if (li.classList.contains('types')) {
        typeLabel = li.textContent.trim();
      } else {
        const defEl = li.querySelector('.def');
        if (defEl) {
          let text = '';
          for (const node of defEl.childNodes) {
            if (node.matches?.('.more, .more-toggle, .edit')) break;
            text += node.textContent;
          }
          text = text.trim().replace(/\s+/g, ' ');
          if (text) defs.push(text);
        }
      }
    });
    if (defs.length > 0) {
      groups.push({ type: typeLabel, defs });
    }
  });

  // Layout constants.
  const W = 600, pad = 32, contentW = W - pad * 2;
  const headSize = 22, pronunSize = 14, typeSize = 13, defSize = 15;
  const lineHeight = 1.45;
  const scale = Math.max(2, window.devicePixelRatio || 2);
  const numIndent = 24;

  // Text wrapping.
  const mctx = document.createElement('canvas').getContext('2d');
  const defFont = font(defSize);
  const defMaxW = contentW - numIndent;

  function wrapText(text) {
    mctx.font = defFont;
    const words = text.split(' ');
    const lines = [];
    let line = '';
    for (const word of words) {
      const test = line ? line + ' ' + word : word;
      if (mctx.measureText(test).width > defMaxW && line) {
        lines.push(line);
        line = word;
      } else {
        line = test;
      }
    }
    if (line) lines.push(line);
    return lines.length ? lines : [''];
  }

  // Pre-wrap all definitions.
  const groupLayouts = groups.map((g) => ({
    type: g.type,
    defs: g.defs.map(wrapText),
  }));

  // Unified layout: measures when ctx is null, draws when provided.
  function doLayout(ctx) {
    let y = pad;

    if (ctx) {
      ctx.font = font(headSize, true);
      ctx.fillStyle = colors.primary;
      ctx.textBaseline = 'top';
      ctx.fillText(headword, pad, y);
    }
    y += headSize * lineHeight;

    if (pronun) {
      if (ctx) {
        ctx.font = font(pronunSize);
        ctx.fillStyle = colors.light;
        ctx.fillText(pronun, pad, y);
      }
      y += pronunSize * lineHeight + 2;
    }
    y += 12;

    for (const gl of groupLayouts) {
      if (gl.type) {
        if (ctx) {
          ctx.font = font(typeSize, true);
          ctx.fillStyle = colors.light;
          const tw = ctx.measureText(gl.type).width;
          ctx.fillText(gl.type, pad, y);
          ctx.setLineDash([3, 3]);
          ctx.strokeStyle = colors.lighter;
          ctx.lineWidth = 1;
          ctx.beginPath();
          ctx.moveTo(pad, y + typeSize + 2);
          ctx.lineTo(pad + tw, y + typeSize + 2);
          ctx.stroke();
          ctx.setLineDash([]);
        }
        y += typeSize * lineHeight + 8;
      }

      for (let i = 0; i < gl.defs.length; i++) {
        if (ctx) {
          ctx.font = defFont;
          ctx.fillStyle = colors.lighter;
          ctx.fillText(`${i + 1}.`, pad, y);
          ctx.fillStyle = colors.primary;
        }
        for (const line of gl.defs[i]) {
          if (ctx) ctx.fillText(line, pad + numIndent, y);
          y += defSize * lineHeight;
        }
        y += 4;
      }
      y += 4;
    }

    return y + pad - 4;
  }

  // Measure, create canvas, draw.
  const H = Math.ceil(doLayout(null));
  const canvas = document.createElement('canvas');
  canvas.width = W * scale;
  canvas.height = H * scale;
  const ctx = canvas.getContext('2d');
  ctx.scale(scale, scale);

  // Card background.
  ctx.fillStyle = colors.bg;
  ctx.beginPath();
  ctx.roundRect(4, 4, W - 8, H - 8, 8);
  ctx.fill();
  ctx.strokeStyle = colors.border;
  ctx.lineWidth = 1.2;
  ctx.stroke();

  doLayout(ctx);

  return new Promise((resolve) => {
    canvas.toBlob((blob) => resolve(blob), 'image/png');
  });
}

async function shareEntry(entryEl) {
  const blob = await renderEntryCard(entryEl);
  const head = entryEl.dataset.head || 'entry';
  const filename = `${head.replace(/[^a-zA-Z0-9\u0900-\u097F\u0D00-\u0D7F]/g, '_')}.png`;
  const file = new File([blob], filename, { type: 'image/png', lastModified: Date.now() });

  // 1. Try Web Share with image file.
  if (navigator.canShare && navigator.canShare({ files: [file] })) {
    await navigator.share({ files: [file] });
    return;
  }

  // 2. Try Web Share with text/URL (no file support).
  if (navigator.share) {
    const def = entryEl.querySelector('.def');
    const url = `${window.location.origin}${window.location.pathname}#${entryEl.id}`;
    await navigator.share({
      title: head,
      text: def?.textContent?.trim() || '',
      url: url,
    });
    return;
  }

  // 3. Try clipboard.
  if (navigator.clipboard && window.ClipboardItem) {
    try {
      await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]);
      alert('Screenshot copied to clipboard');
      return;
    } catch (e) {
      console.log('Clipboard write failed, downloading instead:', e);
    }
  }

  // Fallback: download.
  const url = URL.createObjectURL(blob);
  const a = Object.assign(document.createElement('a'), { href: url, download: filename });
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
