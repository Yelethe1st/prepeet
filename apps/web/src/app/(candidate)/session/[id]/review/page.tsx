import { ReviewScreen } from "@/features/review/ReviewScreen";

/**
 * The coaching destination: information-architecture's
 * /candidate/session/[id]/review. The record lives at results; this page
 * is the work that follows from it.
 */
export default async function ReviewPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Coaching review</h1>
          <p className="page-desc">
            What to change and how, built only from what you actually said. The
            outcome itself never changes; this is the work that follows from it.
          </p>
        </div>
      </div>
      <section aria-label="Coaching review" className="mt-6">
        <ReviewScreen sessionId={id} />
      </section>
    </>
  );
}
