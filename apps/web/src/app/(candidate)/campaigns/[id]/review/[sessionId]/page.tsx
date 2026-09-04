import { ReviewScreen } from "@/features/campaigns/ReviewScreen";

/**
 * The review destination: REV-02. Evidence first, decision second, and the
 * decision belongs to the person reading this page. The server records the
 * read before answering it, and refuses when it cannot record.
 */
export default async function ReviewPage({
  params,
}: {
  params: Promise<{ id: string; sessionId: string }>;
}) {
  const { id, sessionId } = await params;
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Screening review</h1>
          <p className="page-desc">
            Evidence first, decision second. Reading this page is recorded in
            the audit log.
          </p>
        </div>
      </div>
      <section aria-label="Screening review" className="mt-6">
        <ReviewScreen campaignId={id} sessionId={sessionId} />
      </section>
    </>
  );
}
