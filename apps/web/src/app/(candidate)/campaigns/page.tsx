import { CampaignsScreen } from "@/features/campaigns/CampaignsScreen";

/**
 * The campaigns destination: every campaign in the workspace, and the way
 * into each one's candidate roster. The shell reveals this to anyone with
 * campaign.read; what a recruiter may do inside a campaign is decided per
 * campaign by the join, on the server.
 */
export default function CampaignsPage() {
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Campaigns</h1>
          <p className="page-desc">
            A campaign fixes one interview configuration, so every candidate it
            invites sits the same interview. Open one to see its roster.
          </p>
        </div>
      </div>
      <section aria-label="Campaigns" className="mt-6">
        <CampaignsScreen />
      </section>
    </>
  );
}
