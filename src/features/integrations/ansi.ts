// Standard 16-color palette tuned for dark backgrounds (One Dark inspired)
const PALETTE16: readonly string[] = [
  '#3c3c3c', // 0  black
  '#e06c75', // 1  red
  '#98c379', // 2  green
  '#e5c07b', // 3  yellow
  '#61afef', // 4  blue
  '#c678dd', // 5  magenta
  '#56b6c2', // 6  cyan
  '#abb2bf', // 7  white
  '#5c6370', // 8  bright black
  '#e06c75', // 9  bright red
  '#98c379', // 10 bright green
  '#e5c07b', // 11 bright yellow
  '#61afef', // 12 bright blue
  '#c678dd', // 13 bright magenta
  '#56b6c2', // 14 bright cyan
  '#ffffff', // 15 bright white
];

function xterm256(n: number): string {
  if (n < 16) {
    return PALETTE16[n];
  }
  if (n >= 232) {
    const v = Math.round(((n - 232) / 23) * 255);
    const h = v.toString(16).padStart(2, '0');
    return `#${h}${h}${h}`;
  }
  const idx = n - 16;
  const r = Math.floor(idx / 36);
  const g = Math.floor((idx % 36) / 6);
  const b = idx % 6;
  const c = (v: number) => (v === 0 ? 0 : v * 40 + 55).toString(16).padStart(2, '0');
  return `#${c(r)}${c(g)}${c(b)}`;
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// Matches any CSI/OSC/two-char ESC sequence so we can selectively process SGR
// and strip everything else cleanly.
const ANSI_RE = /\x1b(?:\[([0-9;]*)m|\[[0-9;]*[A-Za-z@]|\][^\x07\x1b]*(?:\x07|\x1b\\)|\(B|[@-_])/g;

export function ansiToHtml(text: string): string {
  // Drop carriage returns (programs may use \r\n or \r for progress bars)
  const input = text.replace(/\r/g, '');
  const parts: string[] = [];
  let openSpans = 0;
  let last = 0;
  let match: RegExpExecArray | null;

  ANSI_RE.lastIndex = 0;

  while ((match = ANSI_RE.exec(input)) !== null) {
    if (match.index > last) {
      parts.push(escapeHtml(input.slice(last, match.index)));
    }
    last = ANSI_RE.lastIndex;

    // Only the first capture group is set for SGR sequences (group 1 = params)
    if (match[1] === undefined) continue;

    const codes = match[1] === '' ? [0] : match[1].split(';').map(Number);
    let k = 0;

    while (k < codes.length) {
      const code = codes[k];

      if (code === 0) {
        if (openSpans > 0) {
          parts.push('</span>'.repeat(openSpans));
          openSpans = 0;
        }
        k++;
      } else if (code === 1) {
        parts.push('<span style="font-weight:bold">');
        openSpans++;
        k++;
      } else if (code >= 30 && code <= 37) {
        parts.push(`<span style="color:${PALETTE16[code - 30]}">`);
        openSpans++;
        k++;
      } else if (code >= 90 && code <= 97) {
        parts.push(`<span style="color:${PALETTE16[code - 90 + 8]}">`);
        openSpans++;
        k++;
      } else if (code >= 40 && code <= 47) {
        parts.push(`<span style="background-color:${PALETTE16[code - 40]}">`);
        openSpans++;
        k++;
      } else if (code >= 100 && code <= 107) {
        parts.push(`<span style="background-color:${PALETTE16[code - 100 + 8]}">`);
        openSpans++;
        k++;
      } else if ((code === 38 || code === 48) && codes[k + 1] === 5 && k + 2 < codes.length) {
        const prop = code === 38 ? 'color' : 'background-color';
        parts.push(`<span style="${prop}:${xterm256(codes[k + 2])}">`);
        openSpans++;
        k += 3;
      } else if ((code === 38 || code === 48) && codes[k + 1] === 2 && k + 4 < codes.length) {
        const prop = code === 38 ? 'color' : 'background-color';
        parts.push(`<span style="${prop}:rgb(${codes[k + 2]},${codes[k + 3]},${codes[k + 4]})">`);
        openSpans++;
        k += 5;
      } else {
        k++;
      }
    }
  }

  if (last < input.length) {
    parts.push(escapeHtml(input.slice(last)));
  }
  if (openSpans > 0) {
    parts.push('</span>'.repeat(openSpans));
  }

  return parts.join('');
}
