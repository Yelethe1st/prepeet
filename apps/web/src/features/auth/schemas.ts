import { z } from "zod";

/**
 * What the browser checks before asking the server.
 *
 * The server is still the authority. The contract declares what is accepted and
 * the server enforces it, and the forms render the `field_errors` it sends back,
 * so nothing here can permit something the server refuses.
 *
 * What these are for is immediacy: a round trip to be told an address is
 * missing is unkind, and on a slow connection it is a wait for nothing.
 *
 * # Why these rules and no more
 *
 * Deliberately the weakest checks that catch a genuine mistake. Anything
 * stricter would be a second definition of what is valid, and the one that
 * drifts would be the browser telling somebody their input is fine when the
 * server will not take it, or refusing something the server would.
 *
 * The password minimum is the one number repeated from the server, and it is
 * repeated rather than derived because the contract expresses it as a schema
 * constraint that does not survive into the generated TypeScript. A test asserts
 * the two agree.
 */

/** The shortest password the server accepts. Mirrored from identity.ValidatePassword. */
export const MINIMUM_PASSWORD_LENGTH = 12;

/**
 * An address is checked for shape only.
 *
 * Whether it can be delivered to is not something a regular expression knows,
 * and every attempt to decide it in the browser refuses somebody's real address.
 * The server normalises and validates; this only catches an obvious typo.
 */
const email = z
  .string()
  .min(1, "Enter your email address.")
  .email("That does not look like an email address.");

export const signInSchema = z.object({
  email,
  // No length rule on sign-in. An existing password that no longer meets a
  // current rule must still be usable to sign in, or a policy change locks
  // people out of their own accounts.
  password: z.string().min(1, "Enter your password."),
});

export type SignInValues = z.infer<typeof signInSchema>;

export const registerSchema = z
  .object({
    email,
    password: z
      .string()
      .min(
        MINIMUM_PASSWORD_LENGTH,
        `A password needs at least ${MINIMUM_PASSWORD_LENGTH} characters.`,
      ),
    accountType: z.enum(["candidate", "organisation"]),
    organisationName: z.string().optional(),
  })
  .refine(
    (values) => values.accountType !== "organisation" || (values.organisationName ?? "").trim() !== "",
    {
      // Reported against the field it belongs to, so it appears next to the
      // input rather than at the top of the form where somebody has to work out
      // which control it means.
      path: ["organisationName"],
      message: "Give the name candidates will recognise.",
    },
  );

export type RegisterValues = z.infer<typeof registerSchema>;

/**
 * Recovery asks only for the address; everything else the server decides.
 */
export const forgotPasswordSchema = z.object({
  email: z.string().trim().toLowerCase().email("Enter a valid email address."),
});

export type ForgotPasswordInput = z.infer<typeof forgotPasswordSchema>;

/**
 * The new password, entered twice.
 *
 * The rules stated here are exactly the rules the server enforces: length and
 * agreement. The prototype's requirement list also claimed mixed case, digits
 * and breach-list checks, which the backend does not enforce; promising a
 * check that does not happen teaches people the wrong thing about what made
 * their password acceptable, so those lines did not port.
 */
export const resetPasswordSchema = z
  .object({
    password: z
      .string()
      .min(
        MINIMUM_PASSWORD_LENGTH,
        `A password needs at least ${MINIMUM_PASSWORD_LENGTH} characters.`,
      ),
    confirm: z.string(),
  })
  .refine((value) => value.password === value.confirm, {
    path: ["confirm"],
    message: "The two passwords are not the same.",
  });

export type ResetPasswordInput = z.infer<typeof resetPasswordSchema>;

/**
 * Six digits, exactly. The pattern mirrors the contract's, so a code the
 * server would refuse is refused before the request.
 */
export const otpSchema = z.object({
  code: z.string().regex(/^[0-9]{6}$/, "The code is the six digits from the email."),
});

export type OtpInput = z.infer<typeof otpSchema>;
