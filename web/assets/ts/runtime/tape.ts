// The event tape: the system-wide success idiom (spec §4 / craft-ceiling
// graft). Timestamped uppercase instrument-printout lines, newest first,
// max 3 retained, at most one action per line, no auto-dismiss. Rendered
// into the page's `ui/tape` region via DOM building (textContent only --
// never markup from data), with a 150ms draw-in on the newest line that
// the global reduced-motion override suppresses.

const MAX_LINES = 3;

export interface TapeAction {
  label: string;
  run: () => void;
}

function timeStamp(): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

/**
 * Prints one line onto the page's tape region. A page without a tape
 * region (`ui/tape` not included) drops the line silently -- the tape is
 * a feedback surface, never load-bearing state.
 */
export function printTape(text: string, action?: TapeAction): void {
  const tape = document.getElementById("event-tape");
  if (!tape) return;

  const line = document.createElement("p");
  line.className = "tape__line tape__line--new";

  const time = document.createElement("span");
  time.className = "tape__time";
  time.textContent = timeStamp();
  line.appendChild(time);

  const body = document.createElement("span");
  body.className = "tape__text";
  body.textContent = text;
  line.appendChild(body);

  if (action) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "tape__action";
    btn.textContent = action.label;
    btn.addEventListener("click", action.run);
    line.appendChild(btn);
  }

  tape.prepend(line);
  while (tape.children.length > MAX_LINES) {
    tape.removeChild(tape.lastChild as Node);
  }
  tape.hidden = false;
}
