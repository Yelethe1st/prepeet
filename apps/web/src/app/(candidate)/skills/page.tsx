import { SkillsSection } from "@/features/skills/SkillsSection";

/**
 * The candidate's competencies over time, and the evidence behind each.
 *
 * Practice only, which is not a setting but a consequence: the reading is
 * derived from the candidate's own practice history, read under their own
 * scope, and no tenant authority reaches it.
 */
export default function SkillsPage() {
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Skills</h1>
          <p className="page-desc">
            What your practice sessions have shown, competency by competency.
            Open any row to read the evidence behind it, with the date it was
            measured and the rubric that judged it.
          </p>
        </div>
      </div>
      <section aria-label="Your competencies" className="mt-6">
        <SkillsSection />
      </section>
    </>
  );
}
