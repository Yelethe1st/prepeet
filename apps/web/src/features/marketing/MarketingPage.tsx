import { Evidence } from "./Evidence";
import { Faq } from "./Faq";
import { Features } from "./Features";
import { FinalCta } from "./FinalCta";
import { Hero } from "./Hero";
import { HowItWorks } from "./HowItWorks";
import { Realtime } from "./Realtime";
import { SiteFooter } from "./SiteFooter";
import { SiteHeader } from "./SiteHeader";
import { UseCases } from "./UseCases";

/**
 * The public front page, ported from screens/index.html under WEB-06.
 *
 * The order is the prototype's, and the order is an argument: what it does,
 * who uses it, what it produces, how it runs, what the conversation is actually
 * like, what the evidence looks like, the questions people are afraid to ask,
 * and then the invitation. Moving a section moves a step in that argument, so
 * the sections are composed here and nowhere else.
 *
 * Two of the prototype's sections are not here. It opens with a logo strip of
 * six named organisations and four usage numbers, and later quotes three named
 * people at two of those organisations. None of them exists. Invented customers
 * and invented counts are a claim this page has no business making, and the
 * page argues perfectly well from what the product actually does: the two
 * guarantees the numbers were standing in for, that a person reviews every
 * screen evaluation and that no score is published without a linked quote, are
 * stated in the visibility table and the evidence section, where they are
 * demonstrated rather than counted. They return when there is somebody real to
 * name.
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
        <UseCases />
        <Features />
        <HowItWorks />
        <Realtime />
        <Evidence />
        <Faq />
        <FinalCta />
      </main>
      <SiteFooter />
    </>
  );
}
