import type { NotificationEvent } from "../api/generated";

/** The events a notification target can subscribe to, in display order. */
export const EVENTS: { event: NotificationEvent; title: string; hint: string }[] = [
  {
    event: "container_status",
    title: "Service problems",
    hint: "A service you manage crashes, goes unhealthy, stops, or comes back up.",
  },
  {
    event: "container_missing",
    title: "Services removed",
    hint: "A container you manage is removed from this machine. Usually that's you, taking it down or editing your compose file, so turn this off if you change things often.",
  },
  {
    event: "container_adopted",
    title: "New services picked up",
    hint: "A container appears in a service Veery already manages, so Veery starts watching it too.",
  },
  {
    event: "update_applied",
    title: "Update results",
    hint: "An update finished, or failed and was rolled back.",
  },
  {
    event: "update_available",
    title: "Updates you can install",
    hint: "A newer version is out for a service that doesn't update itself.",
  },
  {
    event: "auth",
    title: "Sign-ins and passkeys",
    hint: "Someone signs in, or a new passkey is enrolled.",
  },
  {
    event: "raid_unhealthy",
    title: "RAID unhealthy",
    hint: "A RAID array goes degraded or failed, and again when it recovers.",
  },
  {
    event: "raid_disk_offline",
    title: "RAID disk offline",
    hint: "A member disk drops out of a RAID array, and again when it comes back.",
  },
  {
    event: "raid_scan_started",
    title: "RAID scan started",
    hint: "A data-scrub starts on a RAID array, whether scheduled, from a host cron, or run by hand.",
  },
  {
    event: "raid_scan_finished",
    title: "RAID scan finished",
    hint: "A data-scrub finishes and the array returns to idle.",
  },
];
