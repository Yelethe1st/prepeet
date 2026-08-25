import { CvSection } from "@/features/profile/CvSection";

/**
 * The candidate's profile: the CV, and what extraction read from it, span by
 * span, for them to confirm or correct. The rest of the prototype's profile
 * screen - career context, goals, the evidence bank - arrives with its own
 * tickets; this page grows around this section rather than being rebuilt.
 */
export default function ProfilePage() {
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Profile</h1>
          <p className="page-desc">
            Your CV and what we parsed from it. Parsing proposes; you decide -
            every fact shows where it came from, and your word is the one that
            counts.
          </p>
        </div>
      </div>
      <section aria-label="Your CV" className="mt-6">
        <CvSection />
      </section>
    </>
  );
}
