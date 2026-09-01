"use client";

import { useQuery } from "@tanstack/react-query";

import { ApiError } from "@/lib/api/client";
import { Button } from "@/shared/components";
import {
  ErrorState,
  LoadingSurface,
  SkeletonBlock,
  SkeletonText,
} from "@/shared/states";

import { getSkills } from "./api";
import { SkillsScreen } from "./SkillsScreen";

/**
 * Loads the competency history and hands it to the screen.
 *
 * Split from SkillsScreen so the screen is a pure function of its data. The
 * three things PRG-04 asks for are all properties of how the reading is
 * rendered, and testing them through a fetch would mean every one of those
 * tests could fail for a reason that has nothing to do with them.
 */
export function SkillsSection() {
  const skills = useQuery({ queryKey: ["skills"], queryFn: getSkills });

  if (skills.isPending) {
    return (
      <LoadingSurface label="your competencies">
        <SkeletonText width="50" />
        <SkeletonBlock />
      </LoadingSurface>
    );
  }

  if (skills.isError) {
    return (
      <ErrorState
        what="Your competencies could not be loaded"
        safe="Nothing about your practice history has changed; only this view failed."
        reference={
          skills.error instanceof ApiError && skills.error.requestId
            ? skills.error.requestId
            : "none"
        }
        action={
          <Button type="button" onClick={() => void skills.refetch()}>
            Try again
          </Button>
        }
      />
    );
  }

  return <SkillsScreen history={skills.data} />;
}
