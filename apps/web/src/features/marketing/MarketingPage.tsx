import { Evidence } from "./Evidence";
import { Faq } from "./Faq";
import { Features } from "./Features";
import { FinalCta } from "./FinalCta";
import { Hero } from "./Hero";
import { HowItWorks } from "./HowItWorks";
import { Proof } from "./Proof";
import { Quotes } from "./Quotes";
import { Realtime } from "./Realtime";
import { SiteFooter } from "./SiteFooter";
import { SiteHeader } from "./SiteHeader";
import { UseCases } from "./UseCases";

/**
 * The public front page, ported from screens/index.html under WEB-06.
 *
 * The order is the prototype's, and the order is an argument: what it does,
 * who uses it, what it produces, how it runs, what the conversation is actually
 * like, what the evidence looks like, who says it works, the questions people
 * are afraid to ask, and then the invitation. Moving a section moves a step in
 * that argument, so the sections are composed here and nowhere else.
 *
 * The skip link is here rather than in the layout because this is the only
 * route with a header to skip past. Everything else in the public group is a
 * single card.
 */
export function MarketingPage() {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to main content
      </a>
      <SiteHeader />
      <main id="main-content">
        <Hero />
        <Proof />
        <UseCases />
        <Features />
        <HowItWorks />
        <Realtime />
        <Evidence />
        <Quotes />
        <Faq />
        <FinalCta />
      </main>
      <SiteFooter />
    </>
  );
}
