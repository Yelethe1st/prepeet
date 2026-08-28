import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axe } from "vitest-axe";

import { Field } from "./Field";

/**
 * The form field, ported from the `.field` block in the prototype.
 *
 * What is asserted here is the part a person depends on and a screenshot does
 * not show: that the label names the input, that a hint and an error are
 * announced with it, and that an invalid field says so to assistive technology
 * rather than only turning red.
 */

describe("Field", () => {
  it("labels the input, so clicking the label focuses it", () => {
    render(
      <Field label="Work or personal email" name="email">
        {(props) => <input {...props} type="email" />}
      </Field>,
    );

    expect(screen.getByLabelText("Work or personal email")).toBeInTheDocument();
  });

  it("associates a hint with the input rather than leaving it floating", () => {
    render(
      <Field label="Password" name="password" hint="At least 12 characters.">
        {(props) => <input {...props} type="password" />}
      </Field>,
    );

    expect(screen.getByLabelText("Password")).toHaveAccessibleDescription(
      "At least 12 characters.",
    );
  });

  /**
   * Colour alone is not an error message. A field that is red to a sighted
   * person and unremarkable to a screen reader fails the accessibility
   * commitment in ACC-01, and is also useless to anyone who cannot see the
   * border.
   */
  it("marks an invalid field invalid, not merely red", () => {
    render(
      <Field
        label="Email"
        name="email"
        error="Enter the email address you registered with."
      >
        {(props) => <input {...props} type="email" />}
      </Field>,
    );

    const input = screen.getByLabelText("Email");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription(
      /Enter the email address you registered with/,
    );
  });

  it("does not claim a field is invalid when it is not", () => {
    render(
      <Field label="Email" name="email">
        {(props) => <input {...props} type="email" />}
      </Field>,
    );

    expect(screen.getByLabelText("Email")).not.toHaveAttribute(
      "aria-invalid",
      "true",
    );
  });

  /**
   * An error replacing the hint would remove the instruction at exactly the
   * moment somebody needs it, since the usual reason a field is invalid is
   * that they did not follow it.
   */
  it("keeps the hint when an error appears", () => {
    render(
      <Field
        label="Password"
        name="password"
        hint="At least 12 characters."
        error="Too short."
      >
        {(props) => <input {...props} type="password" />}
      </Field>,
    );

    const description =
      screen.getByLabelText("Password").getAttribute("aria-describedby") ?? "";
    expect(description.split(" ")).toHaveLength(2);
    expect(screen.getByText("At least 12 characters.")).toBeInTheDocument();
    expect(screen.getByText("Too short.")).toBeInTheDocument();
  });

  /**
   * An error that appears silently is an error a screen reader user does not
   * know about until they navigate back to the field.
   */
  it("announces an error when it appears", () => {
    render(
      <Field label="Email" name="email" error="Not deliverable.">
        {(props) => <input {...props} type="email" />}
      </Field>,
    );

    expect(screen.getByText("Not deliverable.")).toHaveAttribute(
      "role",
      "alert",
    );
  });

  /**
   * The label, hint and error each render as their own element.
   *
   * Not asserted by class name: the styling moved to Tailwind utilities, and a
   * test naming those would fail on a restyle and pass on a broken component.
   * What matters structurally is that the three are separate elements, because
   * that is what lets each be referenced independently by aria-describedby.
   */
  it("renders the label, hint and error as separate elements", () => {
    render(
      <Field label="Email" name="email" hint="A hint." error="An error.">
        {(props) => <input {...props} type="email" />}
      </Field>,
    );

    const hint = screen.getByText("A hint.");
    const error = screen.getByText("An error.");

    expect(hint).not.toBe(error);
    expect(hint.id).not.toBe(error.id);
    expect(screen.getByLabelText("Email")).toHaveAccessibleDescription(
      /A hint\..*An error\./,
    );
  });

  it("has no accessibility violations", async () => {
    const { container } = render(
      <Field label="Email" name="email" hint="A hint." error="An error.">
        {(props) => <input {...props} className="input" type="email" />}
      </Field>,
    );

    expect(await axe(container)).toHaveNoViolations();
  });
});

/**
 * The prototype puts "Forgot your password?" on the same line as the password
 * label. That is a link inside a label's row, not inside the label, and the
 * difference matters: a link inside a label is announced as part of the field's
 * name and steals the click that should focus the input.
 */
describe("Field with a label action", () => {
  it("renders the action beside the label rather than inside it", () => {
    render(
      <Field
        label="Password"
        name="password"
        labelAction={<a href="/forgot">Forgot your password?</a>}
      >
        {(props) => <input {...props} className="input" type="password" />}
      </Field>,
    );

    const link = screen.getByRole("link", { name: "Forgot your password?" });
    expect(link).toBeInTheDocument();
    // The accessible name of the field is the label alone.
    expect(screen.getByLabelText("Password")).toBeInTheDocument();
    expect(link.closest("label")).toBeNull();
  });

  it("has no accessibility violations with an action", async () => {
    const { container } = render(
      <Field
        label="Password"
        name="password"
        labelAction={<a href="/forgot">Forgot?</a>}
      >
        {(props) => <input {...props} className="input" type="password" />}
      </Field>,
    );

    expect(await axe(container)).toHaveNoViolations();
  });
});
