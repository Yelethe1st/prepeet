import { CompleteScreen } from "@/features/complete/CompleteScreen";

/**
 * The completion destination: information-architecture's
 * /candidate/session/[id]/complete. Durable by construction: everything
 * shown is the server's session read, so returning later answers the same.
 */
export default async function CompletePage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Session finished</h1>
          <p className="page-desc">
            What happens now, and where the results appear when they are
            ready.
          </p>
        </div>
      </div>
      <section aria-label="Completion status" className="mt-6">
        <CompleteScreen sessionId={id} />
      </section>
    </>
  );
}
