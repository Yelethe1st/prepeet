import { RosterScreen } from "@/features/campaigns/RosterScreen";

/**
 * The candidate roster destination: REV-01. Who the campaign invited and
 * where each candidate stands, filtered by the server and never ranked.
 * Access is the campaign join: a recruiter not on this campaign gets the
 * same absence a wrong identifier does.
 */
export default async function RosterPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Candidates</h1>
          <p className="page-desc">
            Each row summarises where one candidate stands. It is not a ranking,
            and Prepeet does not recommend who to hire.
          </p>
        </div>
      </div>
      <section aria-label="Candidate roster" className="mt-6">
        <RosterScreen campaignId={id} />
      </section>
    </>
  );
}
