import { PrepareScreen } from "@/features/prepare/PrepareScreen";

/**
 * The prepare destination: information-architecture's
 * /candidate/session/[id]/prepare, with the group prefix the app drops.
 * The page's whole job is handing the session id to the screen.
 */
export default async function PreparePage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Prepare</h1>
          <p className="page-desc">
            Read the brief, check your microphone, then start when you are
            ready. Nothing is recorded until you press start.
          </p>
        </div>
      </div>
      <section aria-label="Preparation" className="mt-6">
        <PrepareScreen sessionId={id} />
      </section>
    </>
  );
}
