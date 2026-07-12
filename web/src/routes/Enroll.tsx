import { Placeholder } from "./Placeholder";

export function Enroll({ token }: { token: string }) {
  return (
    <Placeholder title="Set up your account">
      {token
        ? `Enrollment token accepted (${token.slice(0, 8)}…). Create your passkey to finish.`
        : "This enrollment link is missing its token."}
    </Placeholder>
  );
}
