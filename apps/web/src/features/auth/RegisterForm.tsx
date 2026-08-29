"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm, useWatch } from "react-hook-form";

import { Banner, Button, Field, Input } from "@/shared/components";
import { ApiError } from "@/lib/api/client";
import type { Registration } from "@/lib/auth/api";

import type { RegisterValues } from "./schemas";
import { registerSchema } from "./schemas";

export interface RegisterFormProps {
  register: (registration: Registration) => Promise<void>;
  /** Called once the request is accepted, with the address that was used. */
  onRegistered: (email: string) => void;
}

/** Where a server field name differs from the form's, mapped once. */
const serverFields: Record<string, keyof RegisterValues> = {
  email: "email",
  password: "password",
  account_type: "accountType",
  organisation_name: "organisationName",
};

/**
 * The registration form, ported from screens/register.html.
 *
 * Two branches, as the prototype has: a candidate registering for themselves,
 * and somebody setting up a workspace for their organisation. The second is
 * what creates the tenant and the owning membership, in one transaction, so
 * from here it is still one request.
 *
 * Registration never signs anybody in. Verification comes first, so success is
 * a message rather than a navigation, and the message is the same whether or
 * not the address already had an account: the server answers identically so
 * that nobody can use this to discover who practises for interviews, and a form
 * that said "welcome" only for new accounts would give that away regardless.
 */
export function RegisterForm({
  register: submitRegistration,
  onRegistered,
}: RegisterFormProps) {
  const [failure, setFailure] = useState<ApiError | null>(null);
  const [accepted, setAccepted] = useState(false);
  const [registeredEmail, setRegisteredEmail] = useState("");

  const {
    register,
    handleSubmit,
    control,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<RegisterValues>({
    resolver: zodResolver(registerSchema),
    defaultValues: { accountType: "candidate" },
    mode: "onSubmit",
  });

  /**
   * useWatch rather than watch.
   *
   * watch subscribes the whole component to every field, so a keystroke in the
   * password re-renders the branch that only cares about the account type. It
   * is also opaque to the React compiler, which skips the component entirely
   * and says so as a warning.
   */
  const accountType = useWatch({ control, name: "accountType" });

  async function submit(values: RegisterValues) {
    setFailure(null);

    try {
      await submitRegistration({
        email: values.email,
        password: values.password,
        account_type: values.accountType,
        ...(values.accountType === "organisation"
          ? { organisation_name: values.organisationName }
          : {}),
      });
      setRegisteredEmail(values.email);
      setAccepted(true);
      onRegistered(values.email);
    } catch (error) {
      const failed =
        error instanceof ApiError
          ? error
          : new ApiError({
              status: 0,
              message: "Something went wrong. Please try again.",
            });

      // The server names its fields as the contract does; the form names them
      // as the form does. Mapped here so a field error lands on the control it
      // is about rather than in a banner.
      for (const [field, message] of Object.entries(failed.fieldErrors)) {
        const target = serverFields[field];
        if (target) setError(target, { type: "server", message });
      }

      setFailure(failed);
    }
  }

  if (accepted) {
    return (
      <Banner tone="success">
        <strong>Check your email.</strong>
        <p className="mt-1 leading-relaxed">
          If we can reach <span className="font-mono">{registeredEmail}</span>,
          a link to confirm the address is on its way. The link expires in an
          hour.
        </p>
      </Banner>
    );
  }

  const hasFieldErrors = Boolean(
    errors.email ?? errors.password ?? errors.organisationName,
  );

  return (
    <form onSubmit={(event) => void handleSubmit(submit)(event)} noValidate>
      {failure && !hasFieldErrors ? (
        <Banner tone="danger">
          <strong>{failure.message}</strong>
          {failure.requestId ? (
            <p className="mt-1 text-xs text-fg-3">
              If you contact us, quote{" "}
              <span className="font-mono">{failure.requestId}</span>.
            </p>
          ) : null}
        </Banner>
      ) : null}

      <fieldset className="mb-[18px] border-0 p-0">
        <legend className="mb-2 text-sm font-semibold text-fg">
          What brings you here?
        </legend>

        {/*
          Spaced rather than stacked tight. WCAG 2.2 requires a target of at
          least 24px, or enough space that a 24px circle centred on each does
          not touch the next. The control is 18px, so the spacing exception is
          what this satisfies; with no gap the centres were 19.5px apart and
          failed. Found by the browser suite, which is the only tier that can
          measure it.
        */}
        <div className="flex flex-col gap-4">
          {(
            [
              ["candidate", "Practise interviews for myself"],
              ["organisation", "Screen candidates for my organisation"],
            ] as const
          ).map(([value, label]) => (
            <label
              className="flex cursor-pointer items-start gap-2.5 text-sm text-fg"
              key={value}
            >
              <input
                type="radio"
                value={value}
                className="mt-0.5 h-[18px] w-[18px] flex-none accent-primary"
                {...register("accountType")}
              />
              <span>{label}</span>
            </label>
          ))}
        </div>
      </fieldset>

      <Field
        label="Work or personal email"
        name="email"
        error={errors.email?.message}
      >
        {(props) => (
          <Input
            {...props}
            {...register("email")}
            type="email"
            autoComplete="username"
            placeholder="daniel.okonkwo@example.com"
          />
        )}
      </Field>

      <Field
        label="Password"
        name="password"
        hint="At least 12 characters. Length matters more than symbols."
        error={errors.password?.message}
      >
        {(props) => (
          <Input
            {...props}
            {...register("password")}
            type="password"
            autoComplete="new-password"
          />
        )}
      </Field>

      {accountType === "organisation" ? (
        <Field
          label="Organisation name"
          name="organisation_name"
          hint="Shown to candidates you invite, so use the name they will recognise."
          error={errors.organisationName?.message}
        >
          {(props) => (
            <Input
              {...props}
              {...register("organisationName")}
              type="text"
              autoComplete="organization"
            />
          )}
        </Field>
      ) : null}

      {/*
        ADR-0018's copy rule, at the one surface where a candidate meets both
        modes today: this radio group is where somebody chooses between
        practising and screening, and it is where the fear the ADR is about
        arrives. One brand is only defensible if the isolation is stated where
        the two meet, and the prototype states it here. The port had dropped it.

        Only for a candidate. Somebody creating a workspace is not the person
        being reassured, and saying it to them would read as a limitation of
        what they are buying rather than a promise to the people they screen.
      */}
      {accountType === "candidate" ? (
        <p className="mb-4 text-xs leading-[1.55] text-fg-2">
          Practice sessions are recorded and transcribed so they can be
          reviewed. The recordings are visible to you alone: no employer can see
          them, and practising is never mentioned in a screening interview.
        </p>
      ) : null}

      <Button
        type="submit"
        variant="primary"
        size="lg"
        block
        busy={isSubmitting}
        busyLabel="Creating…"
      >
        {accountType === "organisation"
          ? "Create organisation workspace"
          : "Create account"}
      </Button>
    </form>
  );
}
