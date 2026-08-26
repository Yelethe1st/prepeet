import { ResultsScreen } from "@/features/results/ResultsScreen";

/**
 * The outcome destination: information-architecture's
 * /candidate/session/[id]/results. The page hands the session id to the
 * screen; every judgment shown is the server's.
 */
export default async function ResultsPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Outcome and evidence</h1>
          <p className="page-desc">
            The record of this session: each competency&apos;s result, the
            exact sentences behind it, and the transcript. This page is
            read-only and never changes.
          </p>
        </div>
      </div>
      <section aria-label="Outcome and evidence" className="mt-6">
        <ResultsScreen sessionId={id} />
      </section>
    </>
  );
}
