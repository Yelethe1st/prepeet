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
