"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { ApiError } from "@/lib/api/client";
import { Button } from "@/shared/components";
import {
  ErrorState,
  LoadingSurface,
  SkeletonBlock,
  SkeletonText,
} from "@/shared/states";

import { getSettings, saveSettings } from "./api";
import type { TenantSettingsDocument } from "./api";
import { SettingsScreen } from "./SettingsScreen";

/**
 * Loads the configuration and hands it to the screen.
 *
 * Split so the screen is a pure function of its data. Whether somebody sees
 * controls is the thing this ticket is about, and testing it through a fetch
 * would mean those tests could fail for reasons that have nothing to do with
 * authority.
 */
export function SettingsSection() {
  const client = useQueryClient();
  const settings = useQuery({
    queryKey: ["tenant-settings"],
    queryFn: getSettings,
  });

  const save = useMutation({
    mutationFn: ({
      version,
      document,
    }: {
      version: number;
      document: TenantSettingsDocument;
    }) => saveSettings(version, document),
    onSuccess: (saved) => {
      // The saved document, at its new version, becomes what the screen is
      // editing. Refetching instead would briefly show the old version and
      // invite a second save against it.
      client.setQueryData(["tenant-settings"], saved);
    },
  });

  if (settings.isPending) {
    return (
      <LoadingSurface label="your workspace settings">
        <SkeletonText width="50" />
        <SkeletonBlock />
      </LoadingSurface>
    );
  }

  if (settings.isError) {
    return (
      <ErrorState
        what="The workspace settings could not be loaded"
        safe="Nothing about how this workspace is configured has changed; only this view failed."
        reference={
          settings.error instanceof ApiError && settings.error.requestId
            ? settings.error.requestId
            : "none"
        }
        action={
          <Button type="button" onClick={() => void settings.refetch()}>
            Try again
          </Button>
        }
      />
    );
  }

  return (
    <SettingsScreen
      settings={settings.data}
      saving={save.isPending}
      conflicted={save.error instanceof ApiError && save.error.status === 409}
      onSave={(version, document) => save.mutate({ version, document })}
    />
  );
}
