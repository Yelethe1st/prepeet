import type { Metadata } from "next";

import { MarketingPage } from "@/features/marketing/MarketingPage";

/**
 * The prototype's own title and description, which are the page's first copy
 * and the only copy most people will ever see: they are what a search result
 * and a shared link show.
 */
export const metadata: Metadata = {
  title: "Prepeet · Voice interviews that leave evidence",
  description:
    "Prepeet runs realtime voice interviews in two modes: practice with honest coaching for candidates, and recruiter-controlled screens where every competency score is traceable to something the candidate actually said.",
};

/**
 * The marketing route, ported from screens/index.html under WEB-06.
 *
 * It composes and does nothing else. The page is in `features/marketing`, where
 * it can be rendered by a test without a router.
 */
export default function Page() {
  return <MarketingPage />;
}
