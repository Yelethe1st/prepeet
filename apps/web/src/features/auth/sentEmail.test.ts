import { beforeEach, describe, expect, it } from "vitest";

import { maskEmail, readSentEmail, rememberSentEmail } from "./sentEmail";

describe("sentEmail storage", () => {
  beforeEach(() => sessionStorage.clear());

  it("round-trips what was sent", () => {
    rememberSentEmail({
      kind: "password_reset",
      email: "amara.eze@example.com",
    });
    expect(readSentEmail()).toEqual({
      kind: "password_reset",
      email: "amara.eze@example.com",
    });
  });

  it("returns null for an empty tab", () => {
    expect(readSentEmail()).toBeNull();
  });

  it("returns null rather than a broken object for corrupt storage", () => {
    sessionStorage.setItem("prepeet.sent-email", "{not json");
    expect(readSentEmail()).toBeNull();

    sessionStorage.setItem("prepeet.sent-email", JSON.stringify({ email: 7 }));
    expect(readSentEmail()).toBeNull();
  });
});

describe("maskEmail", () => {
  it("keeps the first and last letter and the domain", () => {
    // The prototype's shape: recognisable to its owner, useless to a shoulder.
    expect(maskEmail("daniel.okonkwo@example.com")).toBe(
      "d•••••••••o@example.com",
    );
  });

  it("does not reveal a short local part by masking nothing", () => {
    expect(maskEmail("ab@example.com")).toBe("a•@example.com");
  });

  it("leaves something unaddress-like alone rather than corrupting it", () => {
    expect(maskEmail("not-an-address")).toBe("not-an-address");
  });
});
