import { Wizard } from "@/features/interview/Wizard";

/**
 * The practice configuration wizard's destination. The URL's query carries
 * the step and every choice, so this page's whole job is to hand them to the
 * wizard: a copied or reloaded link restores exactly where the person was.
 */
export default async function NewPracticeInterviewPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const parameters = await searchParams;
  const single = (key: string): string | undefined => {
    const value = parameters[key];
    return typeof value === "string" ? value : undefined;
  };

  const step = Number(single("step") ?? "1");
  const selection: Record<string, string> = {};
  for (const key of ["role", "shape", "persona", "minutes"]) {
    const value = single(key);
    if (value) {
      selection[key] = value;
    }
  }

  return (
    <>
      <div className="page-header">
        <div>
          <h1>Start a practice interview</h1>
          <p className="page-desc">
            Role, shape, interviewer and length - every option comes from the
            catalogue, and your choices are checked again on the server before
            anything is composed.
          </p>
        </div>
      </div>
      <section aria-label="Configuration" className="mt-6">
        <Wizard
          initialStep={Number.isNaN(step) ? 1 : step}
          initialSelection={selection}
        />
      </section>
    </>
  );
}
