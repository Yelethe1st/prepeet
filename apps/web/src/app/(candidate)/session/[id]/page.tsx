import { LiveScreen } from "@/features/live/LiveScreen";

/**
 * The live interview destination: information-architecture's
 * /candidate/session/[id], with the group prefix the app drops. RTC-01's
 * connection shell renders here; the full interview surface arrives with
 * RTC-06 on top of it.
 */
export default async function LivePage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Interview</h1>
        </div>
      </div>
      <section aria-label="Live interview" className="mt-6">
        <LiveScreen sessionId={id} />
      </section>
    </>
  );
}
