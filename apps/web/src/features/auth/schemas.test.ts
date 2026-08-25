import { describe, expect, it } from "vitest";

import {
  MINIMUM_PASSWORD_LENGTH,
  forgotPasswordSchema,
  otpSchema,
  registerSchema,
  resetPasswordSchema,
  signInSchema,
} from "./schemas";

/**
 * Client-side validation, and what it is deliberately not.
 *
 * The server is the authority. These checks exist so that an obvious mistake is
 * caught without a round trip, and every one of them is the weakest rule that
 * catches a genuine error: anything stricter is a second definition of what is
 * valid, and the one that drifts tells somebody their input is fine when it is
 * not.
 */

describe("signing in", () => {
  it("accepts a plausible address and any password", () => {
    const result = signInSchema.safeParse({
      email: "daniel.okonkwo@example.com",
      password: "x",
    });

    expect(result.success).toBe(true);
  });

  /**
   * No length rule on sign-in, and this is the assertion that keeps it that
   * way. An existing password that no longer meets a current rule must still be
   * usable to sign in, or raising the minimum locks people out of their own
   * accounts.
   */
  it("does not impose a length rule on an existing password", () => {
    const result = signInSchema.safeParse({ email: "a@b.co", password: "short" });

    expect(result.success).toBe(true);
  });

  it("catches a missing address before a round trip", () => {
    const result = signInSchema.safeParse({ email: "", password: "x" });

    expect(result.success).toBe(false);
  });

  it("catches something that is not an address", () => {
    const result = signInSchema.safeParse({ email: "not-an-address", password: "x" });

    expect(result.success).toBe(false);
  });
});

describe("registering", () => {
  it("accepts a candidate with a long enough password", () => {
    const result = registerSchema.safeParse({
      email: "daniel.okonkwo@example.com",
      password: "a-long-enough-password",
      accountType: "candidate",
    });

    expect(result.success).toBe(true);
  });

  it("refuses a password shorter than the server accepts", () => {
    const result = registerSchema.safeParse({
      email: "a@b.co",
      password: "x".repeat(MINIMUM_PASSWORD_LENGTH - 1),
      accountType: "candidate",
    });

    expect(result.success).toBe(false);
  });

  it("accepts one exactly at the minimum, so the boundary is not off by one", () => {
    const result = registerSchema.safeParse({
      email: "a@b.co",
      password: "x".repeat(MINIMUM_PASSWORD_LENGTH),
      accountType: "candidate",
    });

    expect(result.success).toBe(true);
  });

  it("does not ask a candidate for an organisation name", () => {
    const result = registerSchema.safeParse({
      email: "a@b.co",
      password: "a-long-enough-password",
      accountType: "candidate",
      organisationName: "",
    });

    expect(result.success).toBe(true);
  });

  it("requires one from an organisation", () => {
    const result = registerSchema.safeParse({
      email: "a@b.co",
      password: "a-long-enough-password",
      accountType: "organisation",
      organisationName: "",
    });

    expect(result.success).toBe(false);
  });

  /**
   * Reported against the field rather than the form, so it appears next to the
   * input rather than at the top where somebody has to work out which control
   * it means.
   */
  it("reports a missing organisation name against that field", () => {
    const result = registerSchema.safeParse({
      email: "a@b.co",
      password: "a-long-enough-password",
      accountType: "organisation",
      organisationName: "   ",
    });

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0]?.path).toEqual(["organisationName"]);
    }
  });

  it("refuses an account type the contract does not define", () => {
    const result = registerSchema.safeParse({
      email: "a@b.co",
      password: "a-long-enough-password",
      accountType: "administrator",
    });

    expect(result.success).toBe(false);
  });
});

/**
 * The one number repeated from the server.
 *
 * The contract expresses the minimum as a schema constraint that does not
 * survive into the generated TypeScript, so it is written twice. This is what
 * stops the two drifting, and it fails naming both values rather than leaving
 * somebody to find the other one.
 */
describe("the password minimum", () => {
  it("matches the server's", async () => {
    const { readFileSync } = await import("node:fs");
    const { resolve } = await import("node:path");

    const identity = readFileSync(
      resolve(process.cwd(), "../../services/platform/internal/identity/identity.go"),
      "utf8",
    );

    const declared = /minPasswordLength\s*=\s*(\d+)/.exec(identity);

    expect(declared, "minPasswordLength was not found in the identity package").not.toBeNull();
    expect(Number(declared?.[1]), `the browser requires ${MINIMUM_PASSWORD_LENGTH}`).toBe(
      MINIMUM_PASSWORD_LENGTH,
    );
  });
});

describe("forgotPasswordSchema", () => {
  it("normalises the address the way the server will", () => {
    const parsed = forgotPasswordSchema.parse({ email: "  Amara.Eze@Example.COM " });
    expect(parsed.email).toBe("amara.eze@example.com");
  });

  it("refuses a non-address", () => {
    expect(forgotPasswordSchema.safeParse({ email: "not-an-address" }).success).toBe(false);
  });
});

describe("resetPasswordSchema", () => {
  it("holds the new password to the same floor as registration", () => {
    const short = resetPasswordSchema.safeParse({ password: "eleven chars", confirm: "eleven chars" });
    // "eleven chars" is 12 characters, which is exactly the floor.
    expect(short.success).toBe(true);

    const tooShort = resetPasswordSchema.safeParse({ password: "elevenchars", confirm: "elevenchars" });
    expect(tooShort.success).toBe(false);
  });

  it("reports a mismatch against the confirmation field", () => {
    const parsed = resetPasswordSchema.safeParse({
      password: "a long enough password",
      confirm: "a different long password",
    });
    expect(parsed.success).toBe(false);
    if (!parsed.success) {
      expect(parsed.error.issues[0]?.path).toEqual(["confirm"]);
    }
  });
});

describe("otpSchema", () => {
  it("accepts exactly six digits", () => {
    expect(otpSchema.safeParse({ code: "123456" }).success).toBe(true);
  });

  it.each(["12345", "1234567", "12345a", ""])("refuses %j", (code) => {
    expect(otpSchema.safeParse({ code }).success).toBe(false);
  });
});
