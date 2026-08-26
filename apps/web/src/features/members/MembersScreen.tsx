"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { useSession } from "@/lib/auth/session";
import { ApiError } from "@/lib/api/client";
import { Button } from "@/shared/components";
import {
  EmptyState,
  ErrorState,
  LoadingSurface,
  SkeletonText,
} from "@/shared/states";

import {
  changeMemberRole,
  inviteMember,
  listMembers,
  revokeMember,
  type Member,
} from "./api";
import { MATRIX_ROLES, ROLE_LABELS, matrixRows } from "./matrix";

/**
 * The members screen - TEN-02, from the prototype's admin-members screen.
 *
 * A read-only visitor sees everything and can change nothing: hiding the
 * controls is a courtesy on top of the server's refusal, and showing a form
 * that will 403 would be the broken version of honesty. Every change
 * carries the version its row was read at, so two administrators cannot
 * silently overwrite each other, and the permission matrix at the bottom is
 * derived from the capability catalogue - the same artifact the server
 * grants from - rather than typed here.
 */
export function MembersScreen() {
  const session = useSession();
  const manages = session.can("tenant.member_manage");
  const members = useQuery({ queryKey: ["members"], queryFn: listMembers });

  if (members.isPending) {
    return (
      <LoadingSurface label="the members of this workspace">
        <SkeletonText />
        <SkeletonText width="75" />
        <SkeletonText width="50" />
      </LoadingSurface>
    );
  }
  if (members.isError) {
    const failure = members.error;
    return (
      <ErrorState
        what="The member list could not be loaded"
        safe="Memberships themselves are unaffected; only this view failed."
        reference={
          failure instanceof ApiError && failure.requestId
            ? failure.requestId
            : "none"
        }
        action={
          <Button type="button" onClick={() => void members.refetch()}>
            Try again
          </Button>
        }
      />
    );
  }

  return (
    <div className="space-y-8">
      {!manages ? (
        <p className="rounded-md border border-info-border bg-info-soft px-4 py-3 text-sm text-info-fg">
          Inviting people, changing roles and revoking members needs the Tenant
          admin role. You can see the workspace; you cannot change it.
        </p>
      ) : null}

      {members.data.length === 0 ? (
        <EmptyState
          title="Nobody here yet"
          action={manages ? <span /> : <span />}
        >
          Invited members appear here before they accept.
        </EmptyState>
      ) : (
        <MemberTable members={members.data} manages={manages} />
      )}

      {manages ? <InviteForm /> : null}
      <Matrix />
    </div>
  );
}

function MemberTable({
  members,
  manages,
}: {
  members: Member[];
  manages: boolean;
}) {
  return (
    <section aria-label="Members">
      <div className="overflow-x-auto">
        <table className="w-full min-w-[560px] text-left text-sm">
          <caption className="sr-only">The members of this workspace</caption>
          <thead>
            <tr className="border-b border-border text-2xs text-fg-3 uppercase">
              <th scope="col" className="py-2 pr-4">
                Member
              </th>
              <th scope="col" className="py-2 pr-4">
                Role
              </th>
              <th scope="col" className="py-2 pr-4">
                Status
              </th>
              <th scope="col" className="py-2">
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {members.map((member) => (
              <MemberRow
                key={member.membership_id}
                member={member}
                manages={manages}
              />
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function MemberRow({ member, manages }: { member: Member; manages: boolean }) {
  const client = useQueryClient();
  const [problem, setProblem] = useState<string | null>(null);

  const settle = {
    onSuccess: () => {
      setProblem(null);
      void client.invalidateQueries({ queryKey: ["members"] });
    },
    onError: (error: unknown) => {
      setProblem(
        error instanceof ApiError
          ? error.message
          : "That change did not apply. The member is unchanged.",
      );
      // A conflict means the row moved under us; the honest fix is rereading.
      void client.invalidateQueries({ queryKey: ["members"] });
    },
  };
  const change = useMutation({
    mutationFn: (role: string) =>
      changeMemberRole(member.membership_id, role, member.version),
    ...settle,
  });
  const revoke = useMutation({
    mutationFn: () => revokeMember(member.membership_id, member.version),
    ...settle,
  });

  // The owner's row offers no controls to anybody: the anchor role is not
  // this surface's to assign or remove, and a disabled dropdown would
  // suggest otherwise is a permission away.
  const controllable =
    manages && member.role !== "owner" && member.status !== "revoked";

  return (
    <tr
      className={`border-b border-border ${member.status === "revoked" ? "opacity-60" : ""}`}
    >
      <td className="py-2 pr-4">{member.email}</td>
      <td className="py-2 pr-4">
        {controllable ? (
          <label>
            <span className="sr-only">Role for {member.email}</span>
            <select
              className="rounded-md border border-border bg-surface px-2 py-1 text-sm"
              value={member.role}
              onChange={(event) => change.mutate(event.target.value)}
              disabled={change.isPending}
            >
              {MATRIX_ROLES.map((role) => (
                <option key={role} value={role}>
                  {ROLE_LABELS[role]}
                </option>
              ))}
            </select>
          </label>
        ) : (
          <span>
            {member.role === "owner"
              ? "Owner"
              : (ROLE_LABELS[member.role as keyof typeof ROLE_LABELS] ??
                member.role)}
          </span>
        )}
      </td>
      <td className="py-2 pr-4">{member.status}</td>
      <td className="py-2 text-right">
        {controllable ? (
          <Button
            type="button"
            size="sm"
            variant="secondary"
            onClick={() => revoke.mutate()}
            busy={revoke.isPending}
          >
            Revoke
          </Button>
        ) : null}
        {problem ? (
          <p role="alert" className="mt-1 text-left text-sm text-danger">
            {problem}
          </p>
        ) : null}
      </td>
    </tr>
  );
}

function InviteForm() {
  const client = useQueryClient();
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<string>("recruiter");
  const [problem, setProblem] = useState<string | null>(null);

  const invite = useMutation({
    mutationFn: () => inviteMember(email, role),
    onSuccess: () => {
      setEmail("");
      setProblem(null);
      void client.invalidateQueries({ queryKey: ["members"] });
    },
    onError: (error: unknown) => {
      if (error instanceof ApiError) {
        const fieldMessage = Object.values(error.fieldErrors)[0];
        setProblem(fieldMessage ?? error.message);
        return;
      }
      setProblem("The invitation was not sent. Nothing changed.");
    },
  });

  return (
    <section aria-labelledby="invite-heading" className="max-w-[480px]">
      <h2 id="invite-heading" className="text-base font-semibold">
        Invite a member
      </h2>
      <p className="mt-1 text-sm text-fg-2">
        The address must already have a Prepeet account. They accept by opening
        this workspace.
      </p>
      <form
        className="mt-3 space-y-3"
        onSubmit={(event) => {
          event.preventDefault();
          invite.mutate();
        }}
      >
        <div>
          <label className="block text-sm font-semibold" htmlFor="invite-email">
            Email
          </label>
          <input
            id="invite-email"
            type="email"
            required
            className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        </div>
        <div>
          <label className="block text-sm font-semibold" htmlFor="invite-role">
            Role
          </label>
          <select
            id="invite-role"
            className="mt-1 rounded-md border border-border bg-surface px-2 py-1 text-sm"
            value={role}
            onChange={(event) => setRole(event.target.value)}
          >
            {MATRIX_ROLES.map((each) => (
              <option key={each} value={each}>
                {ROLE_LABELS[each]}
              </option>
            ))}
          </select>
        </div>
        <Button type="submit" busy={invite.isPending}>
          Invite
        </Button>
        {problem ? (
          <p role="alert" className="text-sm text-danger">
            {problem}
          </p>
        ) : null}
      </form>
    </section>
  );
}

function Matrix() {
  const rows = matrixRows();
  return (
    <section aria-labelledby="matrix-heading">
      <h2 id="matrix-heading" className="text-base font-semibold">
        What each role can do
      </h2>
      <p className="mt-1 text-sm text-fg-2">
        Generated from the capability catalogue, so this table and the server
        cannot disagree. Scoped means the capability works only within the
        campaigns the person is assigned to.
      </p>
      <div className="mt-3 overflow-x-auto">
        <table
          className="w-full min-w-[660px] text-left text-sm"
          aria-label="Permission matrix"
        >
          <thead>
            <tr className="border-b border-border text-2xs text-fg-3 uppercase">
              <th scope="col" className="py-2 pr-4">
                Capability
              </th>
              {MATRIX_ROLES.map((role) => (
                <th key={role} scope="col" className="py-2 pr-4">
                  {ROLE_LABELS[role]}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr
                key={row.capability}
                className="border-b border-border align-top"
              >
                <th
                  scope="row"
                  className="py-2 pr-4 font-mono text-xs font-normal"
                >
                  {row.capability}
                  <span className="mt-0.5 block max-w-[320px] font-sans text-2xs text-fg-3">
                    {row.reason}
                  </span>
                </th>
                {MATRIX_ROLES.map((role) => (
                  <td key={role} className="py-2 pr-4">
                    {row.cells[role] === "yes"
                      ? "Yes"
                      : row.cells[role] === "scoped"
                        ? "Scoped"
                        : "No"}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
