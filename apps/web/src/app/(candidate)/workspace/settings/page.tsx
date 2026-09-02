import { SettingsSection } from "@/features/settings/SettingsSection";

/**
 * How this workspace is configured.
 *
 * Every membership role may open it; only an administrator or owner meets
 * controls. That difference is decided by the server and rendered here, rather
 * than by this page reasoning about roles.
 */
export default function WorkspaceSettingsPage() {
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Workspace settings</h1>
          <p className="page-desc">
            The name candidates see, and the defaults new campaigns start from.
            Changes apply to campaigns opened afterwards, never to ones already
            running.
          </p>
        </div>
      </div>
      <section aria-label="Workspace settings" className="mt-6">
        <SettingsSection />
      </section>
    </>
  );
}
