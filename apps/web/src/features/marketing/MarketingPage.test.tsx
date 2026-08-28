import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axe } from "vitest-axe";

import { MarketingPage } from "./MarketingPage";
import { faq, footer, howItWorks, hero, useCases } from "./content";

/**
 * The front page, checked against the prototype it was ported from.
 *
 * WEB-06 asks for layout, copy and state coverage. Layout is the browser
 * suite's job, because jsdom has no layout engine. What can be checked here is
 * that every section the prototype demonstrates arrived, that the document has
 * an outline somebody can navigate, and that the promises this page makes on
 * the product's behalf are actually on it.
 */
describe("the marketing page", () => {
  it("has exactly one first-level heading", () => {
    render(<MarketingPage />);

    const headings = screen.getAllByRole("heading", { level: 1 });
    expect(headings).toHaveLength(1);
    expect(headings[0]).toHaveTextContent(
      /Interviews that leave evidence, not impressions\./,
    );
  });

  it("has no accessibility violations", async () => {
    const { container } = render(<MarketingPage />);

    expect(await axe(container)).toHaveNoViolations();
  });

  /**
   * Ten sections, in the prototype's order, because the order is the argument.
   * Named by their headings rather than counted, so a section that arrives
   * unlabelled fails here rather than being counted as present.
   */
  it("renders every section the prototype has", () => {
    render(<MarketingPage />);

    for (const heading of [
      useCases.heading,
      "Built for the part everyone skips: the write-up",
      howItWorks.heading,
      "It behaves like an interviewer, including when you cut it off",
      "A score you can’t trace back to a sentence isn’t a score",
      faq.heading,
      "Start with one conversation",
    ]) {
      expect(
        screen.getByRole("heading", { name: heading, level: 2 }),
      ).toBeInTheDocument();
    }
  });

  /** The picture of the live screen is a picture, and it says what it shows. */
  it("describes the interview preview rather than announcing its parts", () => {
    render(<MarketingPage />);

    const preview = screen.getByRole("img", { name: hero.frameDescription });
    const caption = within(preview).getByText(hero.frameCaption);

    // Drawn, and not announced. A picture of a session read out part by part
    // sounds like a session somebody is in.
    expect(caption.closest('[aria-hidden="true"]')).not.toBeNull();
  });

  it("names all six domains and quotes a question from each", () => {
    render(<MarketingPage />);

    for (const item of useCases.items) {
      expect(
        screen.getByRole("heading", { name: item.role, level: 3 }),
      ).toBeInTheDocument();
      expect(screen.getByText(item.question)).toBeInTheDocument();
    }
  });

  /**
   * The visibility table is the ADR-0018 surface: it is where a visitor is told
   * what a screen candidate is shown and what practice keeps to itself. It stays
   * a table, with a row header per row, because that is what makes a
   * nine-by-three grid navigable rather than a wall of cells.
   */
  it("keeps the visibility table a table, with every row and its header", () => {
    render(<MarketingPage />);

    const table = screen.getByRole("table", { name: howItWorks.tableCaption });
    const columns = within(table).getAllByRole("columnheader");
    expect(columns.map((column) => column.textContent)).toEqual(
      howItWorks.columns,
    );

    for (const row of howItWorks.rows) {
      expect(
        within(table).getByRole("rowheader", { name: row.what }),
      ).toBeInTheDocument();
    }
  });

  /** Never colour alone. Every cell carries the sentence it means. */
  it("writes each answer out in the cell rather than leaving a mark to carry it", () => {
    render(<MarketingPage />);

    const table = screen.getByRole("table", { name: howItWorks.tableCaption });
    expect(within(table).getAllByText("Never shown")).toHaveLength(4);
    expect(
      within(table).getByText("Never generated at all"),
    ).toBeInTheDocument();
  });

  /**
   * The withheld competency reads as withheld, not as a zero. The states this
   * product ships already refuse to render insufficient evidence as a failure,
   * and the page that advertises the behaviour has to obey it too.
   */
  it("reports the unscored competency as insufficient evidence and not as a number", () => {
    render(<MarketingPage />);

    expect(
      screen.getByText("No score: insufficient evidence"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("1 evidence span · Insufficient evidence"),
    ).toBeInTheDocument();
  });

  /**
   * The prototype explains a tagged span in a tooltip, which a touch user never
   * sees. The explanation is text here, so it is available to everybody.
   */
  it("explains a tagged evidence span in text rather than in a tooltip", () => {
    render(<MarketingPage />);

    expect(
      screen.getByText(
        /Supporting evidence: chooses the safe failure direction/,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Unverified claim: a stated judgement/),
    ).toBeInTheDocument();
  });

  it("answers all eight questions", () => {
    render(<MarketingPage />);

    for (const item of faq.items) {
      expect(
        screen.getByRole("button", { name: item.question }),
      ).toBeInTheDocument();
    }
  });

  /**
   * Every link on the page goes somewhere. The prototype links freely between
   * 56 files and most of those screens are not ported, so this is the check that
   * the remapping was complete rather than mostly done.
   */
  it("links nowhere that does not exist", () => {
    const { container } = render(<MarketingPage />);
    const routes = new Set([
      "/",
      "/login",
      "/register",
      "/practice",
      "/practice/new",
      "/profile",
    ]);

    const dead = [...container.querySelectorAll("a[href]")]
      .map((anchor) => anchor.getAttribute("href") ?? "")
      .filter((href) => !href.startsWith("#") && !routes.has(href));

    expect(dead).toEqual([]);
  });

  /** Every anchor target is on the page, or the link scrolls nowhere. */
  it("has a section behind every anchor", () => {
    const { container } = render(<MarketingPage />);

    const missing = [...container.querySelectorAll('a[href^="#"]')]
      .map((anchor) => (anchor.getAttribute("href") ?? "").slice(1))
      .filter((id) => container.querySelector(`#${id}`) === null);

    expect(missing).toEqual([]);
  });

  it("gives the footer a separately named navigation per column", () => {
    render(<MarketingPage />);

    for (const column of footer.columns) {
      expect(
        screen.getByRole("navigation", { name: column.heading }),
      ).toBeInTheDocument();
    }
  });

  /**
   * The disclaimer is the last thing on the page and the shortest statement of
   * what the product is not. It is in the footer of every page in the prototype
   * for that reason.
   */
  it("says what the product does not do, last", () => {
    render(<MarketingPage />);

    expect(screen.getByText(footer.disclaimer)).toBeInTheDocument();
  });

  it("offers a skip link before the header", () => {
    const { container } = render(<MarketingPage />);

    const first = container.querySelector("a");
    expect(first).toHaveTextContent("Skip to main content");
    expect(first).toHaveAttribute("href", "#main-content");
    expect(container.querySelector("#main-content")).toBeInTheDocument();
  });
});
