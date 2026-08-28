"use client";

import { Menu, X } from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";

import { ButtonLink, Icon, ThemeToggle, Wordmark } from "@/shared/components";

import { primaryNav } from "./content";

/**
 * The public site header, ported from `.mk-header`.
 *
 * Sticky, translucent, and collapsing to a menu on a narrow screen, which is
 * where most of the behaviour is. Three things the prototype does in a script
 * at the bottom of the page are properties of the component here:
 *
 *   The burger says whether the menu is open, and its name changes with it.
 *   A button labelled "open" that closes the thing is worse than no label.
 *
 *   Following a link closes the menu. Every link on this page is an anchor to a
 *   section below, so leaving the menu open would scroll the page behind a
 *   panel covering it.
 *
 *   Escape closes it and returns focus to the burger. Without that, dismissing
 *   the menu from the keyboard leaves focus on an element that is now hidden.
 */
export function SiteHeader() {
  const [menuOpen, setMenuOpen] = useState(false);

  useEffect(() => {
    if (!menuOpen) return;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      setMenuOpen(false);
      document.getElementById("marketing-menu-button")?.focus();
    };

    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [menuOpen]);

  return (
    <header
      className="sticky top-0 z-50 border-b border-border backdrop-blur-md"
      style={{ background: "color-mix(in srgb, var(--bg) 82%, transparent)" }}
    >
      <div className="mx-auto flex h-16 max-w-[1180px] items-center gap-3 px-4 md:gap-6 md:px-6">
        <Link
          href="/"
          aria-label="Prepeet home"
          className="flex items-center gap-2.5 text-[18px] font-bold tracking-[-0.02em] text-fg no-underline"
        >
          <Wordmark />
          Prepeet
        </Link>

        <nav aria-label="Primary" className="ml-3 hidden gap-1 lg:flex">
          {primaryNav.map((link) => (
            <a
              key={link.href}
              href={link.href}
              className="rounded-md px-3 py-2 text-sm font-medium text-fg-2 no-underline hover:bg-surface-3 hover:text-fg"
            >
              {link.label}
            </a>
          ))}
        </nav>

        <div className="ml-auto flex items-center gap-2">
          {/*
            Out of the bar on the narrowest phone, and into the menu below.
            At 320px the brand, the toggle, the call to action and the burger
            together are eight pixels wider than the screen, and a header that
            makes the whole document scroll sideways is the one thing every
            screen shares.
          */}
          <span className="hidden sm:inline-flex">
            <ThemeToggle />
          </span>
          {/*
            Wrapped rather than given a class: ButtonLink applies its own
            className last, so one passed in is silently dropped. Hiding it here
            is the honest way to say "not at this width".
          */}
          <span className="hidden lg:inline-flex">
            <ButtonLink href="/login" size="sm" variant="secondary">
              Sign in
            </ButtonLink>
          </span>
          <ButtonLink href="/register" size="sm">
            Get started
          </ButtonLink>
          <button
            type="button"
            id="marketing-menu-button"
            aria-expanded={menuOpen}
            aria-controls="marketing-menu"
            aria-label={
              menuOpen ? "Close navigation menu" : "Open navigation menu"
            }
            onClick={() => setMenuOpen(!menuOpen)}
            className="inline-flex min-h-9 w-9 items-center justify-center rounded-md border border-border-strong bg-surface text-fg-2 hover:bg-surface-3 hover:text-fg lg:hidden"
          >
            <Icon glyph={menuOpen ? X : Menu} />
          </button>
        </div>
      </div>

      <div
        id="marketing-menu"
        hidden={!menuOpen}
        className="border-t border-border bg-surface px-5 pt-3 pb-5 lg:hidden"
      >
        <nav aria-label="Mobile">
          {primaryNav.map((link) => (
            <a
              key={link.href}
              href={link.href}
              onClick={() => setMenuOpen(false)}
              className="block border-b border-border-subtle px-1 py-3 font-semibold text-fg no-underline"
            >
              {link.label}
            </a>
          ))}
        </nav>
        <div className="mt-3.5 flex flex-col gap-2.5">
          <ButtonLink href="/login" variant="secondary" block>
            Sign in
          </ButtonLink>
          <span className="sm:hidden">
            <ThemeToggle withLabel />
          </span>
        </div>
      </div>
    </header>
  );
}
