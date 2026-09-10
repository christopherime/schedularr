// Priority rank among contending blocks -- shared by the guide
// inspector's "50 · 2nd of 5" readout and the blocks list's PRI column.
// It lives here because two surfaces computing this separately would
// eventually disagree about what rank a block holds, and the operator
// has no way to tell which one lied.
//
// Two halves, and BOTH belong here: isContending decides whether a rank
// may be printed at all, priorityRank computes it. A caller that keeps
// its own opinion about the first half is the drift this module exists
// to end -- the guide once printed "50 · 2nd of 5" directly beside its
// own "Disabled" readout that way.

/** The shape both callers already hold: a BlockRecord satisfies it
 * structurally, so neither call site has to re-default `priority`. */
export interface RankPeer {
  readonly enabled: boolean;
  readonly spec: { readonly priority?: number };
}

/** The block being read, as both surfaces already hold it --
 * `disabled_until` is the BlockRecord's optional dark-window end. */
export interface RankSubject {
  readonly enabled: boolean;
  readonly disabled_until?: string;
}

/**
 * Whether this block is contending for airtime right now: enabled, and
 * not sitting in a dark window that has yet to expire.
 *
 * Callers print the bare priority when this is false. A block that is
 * switched off or dark is not in the field, and a rank beside it would
 * announce a contest it is not in -- reachable whenever an
 * already-generated schedule row keeps airing after the block goes dark.
 *
 * A `disabled_until` in the PAST suppresses nothing, and neither does an
 * unparseable one: same reading as the blocks list's DARK UNTIL chip, so
 * the chip and the rank cannot disagree about the same instant. `now` is
 * the server clock (runtime/bus.ts's serverNow), never Date.now().
 */
export function isContending(block: RankSubject, now: number): boolean {
  if (!block.enabled) return false;
  // Absent, unparseable and already-past all land on the same answer, so
  // they share one line: Date.parse("") is NaN.
  const wake = Date.parse(block.disabled_until ?? "");
  return Number.isNaN(wake) || wake <= now;
}

/** Competition rank of `priority` among the ENABLED peers, plus the size
 * of that field. Ties share a rank (two blocks at 80 are both 1st), and
 * the next block down is 3rd -- dense ranking would call it the
 * runner-up when two blocks actually outrank it.
 *
 * `peers` is the same-channel block list; disabled peers are dropped
 * here because they are not currently contending for airtime. A DARK
 * peer stays in the field -- it is defined and it comes back. Pass the
 * whole channel list -- filtering enabled at the call site would
 * double-count nothing but invites the two surfaces to drift.
 *
 * `of: 0` means there is no field to rank against (the blocks fetch
 * failed, or every peer is disabled); callers render the bare priority
 * rather than a "1st of 0". The block itself need not appear in `peers`
 * -- a ghost slot still ranks against the live field. Whether it may be
 * ranked at all is isContending's call, not this one's. */
export function priorityRank(priority: number, peers: readonly RankPeer[]): { rank: number; of: number } {
  const contenders = peers.filter((p) => p.enabled);
  const higher = contenders.filter((p) => (p.spec.priority ?? 0) > priority).length;
  return { rank: higher + 1, of: contenders.length };
}
