/**
 * Who is speaking, from the room's own signal: RTC-06's speaking state.
 *
 * The SFU already measures voice activity per participant; the browser
 * subscribes rather than re-deriving it from audio frames. The reading is
 * deliberately coarse - the interviewer, the candidate, or nobody - because
 * that is exactly what the screen says in words, and a state the UI cannot
 * name is a state it should not track.
 */

/** Who the room hears: the interviewer, the candidate, or nobody. */
export type Speaker = "ai" | "user" | null;

/** The structural surface the observer needs; livekit-client's Room has it. */
export interface SpeakerRoom {
  on(
    event: string,
    handler: (speakers: { identity: string }[]) => void,
  ): unknown;
  off(
    event: string,
    handler: (speakers: { identity: string }[]) => void,
  ): unknown;
}

/**
 * Subscribes to the room's active-speaker changes and reports the coarse
 * reading. The agent's identity is the anchor because it is the one name
 * the browser knows for certain - the candidate joined as a user id the
 * page never sees. The loudest speaker decides when both are talking,
 * which is the SFU's own ordering. Returns the unsubscribe.
 */
export function observeSpeakers(
  room: SpeakerRoom,
  agentIdentity: string,
  onChange: (speaker: Speaker) => void,
): () => void {
  const handler = (speakers: { identity: string }[]): void => {
    if (speakers.length === 0) {
      onChange(null);
      return;
    }
    onChange(speakers[0]?.identity === agentIdentity ? "ai" : "user");
  };
  room.on("activeSpeakersChanged", handler);
  return () => {
    room.off("activeSpeakersChanged", handler);
  };
}
