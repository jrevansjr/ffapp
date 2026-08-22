import { type FormEvent, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { getSettings, updateSettings } from "@/lib/api"
import type { Settings, SettingsUpdate } from "@/lib/types"

const settingsQueryKey = ["settings"] as const
const inputClassName =
  "mt-1.5 w-full rounded-md border border-border bg-card px-3 py-2 text-sm text-foreground outline-none focus:border-neutral-400 focus:ring-2 focus:ring-neutral-200"

interface SettingsFormState {
  sleeperUsername: string
  sleeperLeagueID: string
  sleeperDraftID: string
  pollingEnabled: boolean
  pollingInterval: string
}

function formStateFromSettings(settings: Settings): SettingsFormState {
  return {
    sleeperUsername: settings.sleeper_username,
    sleeperLeagueID: settings.sleeper_league_id,
    sleeperDraftID: settings.sleeper_draft_id,
    pollingEnabled: settings.polling_enabled,
    pollingInterval: String(settings.polling_interval_ms),
  }
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message !== "" ? error.message : fallback
}

function formatTimestamp(timestamp: string): string {
  const date = new Date(timestamp)
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString()
}

/** SettingsForm owns draft form state while TanStack Query owns the saved row. */
function SettingsForm({ settings }: { settings: Settings }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState(() => formStateFromSettings(settings))
  const [validationError, setValidationError] = useState<string | null>(null)

  const saveSettings = useMutation({
    mutationFn: updateSettings,
    onSuccess: (savedSettings) => {
      queryClient.setQueryData(settingsQueryKey, savedSettings)
      setForm(formStateFromSettings(savedSettings))
      setValidationError(null)
    },
  })

  function beginEditing() {
    setValidationError(null)
    saveSettings.reset()
  }

  function updateTextField(
    field: "sleeperUsername" | "sleeperLeagueID" | "sleeperDraftID" | "pollingInterval",
    value: string,
  ) {
    beginEditing()
    setForm((current) => ({ ...current, [field]: value }))
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const intervalText = form.pollingInterval.trim()
    const interval = Number(intervalText)
    if (
      intervalText === "" ||
      !Number.isInteger(interval) ||
      interval < 500 ||
      interval > 60_000
    ) {
      setValidationError("Polling interval must be a whole number between 500 and 60000.")
      saveSettings.reset()
      return
    }

    const update: SettingsUpdate = {
      sleeper_username: form.sleeperUsername,
      sleeper_league_id: form.sleeperLeagueID,
      sleeper_draft_id: form.sleeperDraftID,
      polling_enabled: form.pollingEnabled,
      polling_interval_ms: interval,
    }
    saveSettings.mutate(update)
  }

  return (
    <form className="mt-6 max-w-2xl space-y-6" onSubmit={handleSubmit} noValidate>
      <div className="rounded-lg border border-border bg-card p-5">
        <div className="grid gap-5 sm:grid-cols-2">
          <label className="block text-sm font-medium" htmlFor="sleeper-username">
            Sleeper username
            <input
              className={inputClassName}
              id="sleeper-username"
              onChange={(event) => updateTextField("sleeperUsername", event.target.value)}
              type="text"
              value={form.sleeperUsername}
            />
          </label>

          <label className="block text-sm font-medium" htmlFor="sleeper-league-id">
            League ID
            <input
              className={inputClassName}
              id="sleeper-league-id"
              onChange={(event) => updateTextField("sleeperLeagueID", event.target.value)}
              type="text"
              value={form.sleeperLeagueID}
            />
          </label>

          <label className="block text-sm font-medium" htmlFor="sleeper-draft-id">
            Draft ID
            <input
              className={inputClassName}
              id="sleeper-draft-id"
              onChange={(event) => updateTextField("sleeperDraftID", event.target.value)}
              type="text"
              value={form.sleeperDraftID}
            />
          </label>

          <label className="block text-sm font-medium" htmlFor="polling-interval">
            Polling interval (ms)
            <input
              className={inputClassName}
              id="polling-interval"
              inputMode="numeric"
              max={60_000}
              min={500}
              onChange={(event) => updateTextField("pollingInterval", event.target.value)}
              step={1}
              type="number"
              value={form.pollingInterval}
            />
          </label>
        </div>

        <label className="mt-5 flex items-start gap-3 text-sm" htmlFor="polling-enabled">
          <input
            checked={form.pollingEnabled}
            className="mt-0.5 size-4 rounded border-border accent-neutral-900"
            id="polling-enabled"
            onChange={(event) => {
              beginEditing()
              setForm((current) => ({ ...current, pollingEnabled: event.target.checked }))
            }}
            type="checkbox"
          />
          <span>
            <span className="block font-medium">Enable draft polling</span>
            <span className="mt-0.5 block text-muted-foreground">
              Polling starts in a later milestone after live draft sync is implemented.
            </span>
          </span>
        </label>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <button
          className="rounded-md bg-neutral-900 px-4 py-2 text-sm font-medium text-white hover:bg-neutral-700 disabled:cursor-not-allowed disabled:opacity-60"
          disabled={saveSettings.isPending}
          type="submit"
        >
          {saveSettings.isPending ? "Saving…" : "Save Settings"}
        </button>
        <span className="text-xs text-muted-foreground">
          Last saved {formatTimestamp(settings.updated_at)}
        </span>
      </div>

      <div aria-live="polite" className="min-h-5 text-sm">
        {validationError && (
          <p className="text-red-700" role="alert">
            {validationError}
          </p>
        )}
        {saveSettings.isError && (
          <p className="text-red-700" role="alert">
            {errorMessage(saveSettings.error, "Could not save settings.")}
          </p>
        )}
        {saveSettings.isSuccess && (
          <p className="text-emerald-700" role="status">
            Settings saved.
          </p>
        )}
      </div>
    </form>
  )
}

/** AdminPage loads the persistent settings row before mounting its form. */
export default function AdminPage() {
  const settings = useQuery({
    queryKey: settingsQueryKey,
    queryFn: getSettings,
  })

  return (
    <section aria-labelledby="admin-heading">
      <h1 id="admin-heading" className="text-xl font-semibold">
        Admin
      </h1>
      <p className="mt-2 text-sm text-muted-foreground">
        Configure the Sleeper draft connection stored in your local SQLite database.
      </p>

      {settings.isPending && (
        <p className="mt-6 text-sm text-muted-foreground" role="status">
          Loading settings…
        </p>
      )}

      {settings.isError && (
        <div className="mt-6 max-w-2xl rounded-md border border-red-200 bg-red-50 p-4 text-sm">
          <p className="text-red-800" role="alert">
            {errorMessage(settings.error, "Could not load settings.")}
          </p>
          <button
            className="mt-3 rounded-md border border-red-300 bg-white px-3 py-1.5 font-medium text-red-800 hover:bg-red-100"
            onClick={() => void settings.refetch()}
            type="button"
          >
            Try Again
          </button>
        </div>
      )}

      {settings.data && <SettingsForm settings={settings.data} />}
    </section>
  )
}
