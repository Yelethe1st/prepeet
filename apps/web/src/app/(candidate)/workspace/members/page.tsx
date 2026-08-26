import { MembersScreen } from "@/features/members/MembersScreen";

/**
 * The members destination: who belongs to the workspace, what each role can
 * do, and the administration controls for whoever holds them. The shell's
 * navigation reveals this to anyone with tenant.member_read; the server
 * decides everything else.
 */
export default function MembersPage() {
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Members</h1>
          <p className="page-desc">
            Everyone who opens a candidate&apos;s transcript is recorded in the
            audit log, whatever their role. Access is logged, not just granted.
          </p>
        </div>
      </div>
      <section aria-label="Member administration" className="mt-6">
        <MembersScreen />
      </section>
    </>
  );
}
