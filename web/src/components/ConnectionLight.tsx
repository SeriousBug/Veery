import { css, cx } from "styled-system/css";
import { hstack } from "styled-system/patterns";
import { useLiveData, type ConnectionState } from "../live/LiveData";

const dotBase = css({ w: "2.5", h: "2.5", borderRadius: "full", flexShrink: 0 });

// Static css() literals per state: Panda extracts colors from literal calls, so
// a runtime `bg: someVariable` would compile to a class with no rule behind it.
const pulse = css({ animation: "pulse 2.8s ease-in-out infinite" });
const DOT: Record<ConnectionState, string> = {
  open: css({ bg: "mint.500" }),
  connecting: cx(css({ bg: "sunshine.400" }), pulse),
  closed: cx(css({ bg: "coral.500" }), pulse),
};
const LABEL: Record<ConnectionState, string> = {
  open: "Connected to Veery",
  connecting: "Connecting to Veery…",
  closed: "Disconnected from Veery",
};

/**
 * Live/dead light for the push stream. Green when connected, amber while
 * (re)connecting, red when the stream is down, so a page that looks live but is
 * actually frozen (Veery restarting mid-update, or the host gone) says so.
 */
export function ConnectionLightView({ connection }: { connection: ConnectionState }) {
  const label = LABEL[connection];
  return (
    <span className={hstack({ flexShrink: 0 })} title={label} role="status" aria-label={label}>
      <span className={cx(dotBase, DOT[connection])} />
    </span>
  );
}

export function ConnectionLight() {
  const { connection } = useLiveData();
  return <ConnectionLightView connection={connection} />;
}
