import { DeliveryScreen } from "@/features/delivery/DeliveryScreen";

/**
 * The delivery destination: information-architecture's
 * /candidate/session/[id]/delivery. Measurements, the ten dimensions,
 * the priorities and the drills, every number the calculator's.
 */
export default async function DeliveryPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Delivery</h1>
          <p className="page-desc">
            How the answers landed on a listener: measured from what you said
            and when, judged dimension by dimension, never added up.
          </p>
        </div>
      </div>
      <section aria-label="Delivery" className="mt-6">
        <DeliveryScreen sessionId={id} />
      </section>
    </>
  );
}
