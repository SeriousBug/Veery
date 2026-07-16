import { hstack } from "styled-system/patterns";
import { useLiveData, type ConnectionState } from "../live/LiveData";

// The dot's colour, glow and animation live in index.css (.conn-dot--*): a glow
// keyframe that swaps box-shadow can't be expressed as a static Panda class, and
// keeping all three states' styling in one place makes them easy to compare.
const DOT_CLASS: Record<ConnectionState, string> = {
  open: "conn-dot conn-dot--open",
  connecting: "conn-dot conn-dot--connecting",
  closed: "conn-dot conn-dot--closed",
};
const LABEL: Record<ConnectionState, string> = {
  open: "Connected to Veery",
  connecting: "Reconnecting to Veery…",
  closed: "Disconnected from Veery",
};

/**
 * Live/dead light for the push stream. Green when connected, a breathing amber
 * while reconnecting, red when the stream is down, so a page that looks live but
 * is actually frozen (Veery restarting mid-update, or the host gone) says so.
 */
export function ConnectionLightView({ connection }: { connection: ConnectionState }) {
  const label = LABEL[connection];
  return (
    <span className={hstack({ flexShrink: 0 })} title={label} role="status" aria-label={label}>
      <span className={DOT_CLASS[connection]} />
    </span>
  );
}

export function ConnectionLight() {
  const { connection } = useLiveData();
  return <ConnectionLightView connection={connection} />;
}
