"use client";

import { useState, useRef, useEffect } from "react";
import type { UserConfig } from "@/lib/api";
import { endpoints } from "@/lib/api";
import { Select } from "@/components/ui/Select";
import { Card } from "@/components/ui/Card";
import { SectionHeader } from "@/components/ui/SectionHeader";
import { TextArea } from "@/components/ui/TextArea";
import type { UpdateConfigFn } from "./types";
import { SaveBar } from "./SaveBar";

// Fallback voice list - used when the server is unreachable.
// Prefixes: a=American, b=British, e=Spanish, f=French, h=Hindi,
//           i=Italian, j=Japanese, p=Portuguese, z=Chinese . f/m = female/male
const KOKORO_VOICES_FALLBACK = [
  // ── American Female ──
  { value: "af_alloy", label: "(af_alloy) - American Female . Alloy" },
  { value: "af_aoede", label: "(af_aoede) - American Female . Aoede" },
  { value: "af_bella", label: "(af_bella) - American Female . Bella" },
  { value: "af_heart", label: "(af_heart) - American Female . Heart" },
  { value: "af_jadzia", label: "(af_jadzia) - American Female . Jadzia" },
  { value: "af_jessica", label: "(af_jessica) - American Female . Jessica" },
  { value: "af_kore", label: "(af_kore) - American Female . Kore" },
  { value: "af_nicole", label: "(af_nicole) - American Female . Nicole" },
  { value: "af_nova", label: "(af_nova) - American Female . Nova" },
  { value: "af_river", label: "(af_river) - American Female . River" },
  { value: "af_sarah", label: "(af_sarah) - American Female . Sarah" },
  { value: "af_sky", label: "(af_sky) - American Female . Sky" },
  // ── American Male ──
  { value: "am_adam", label: "(am_adam) - American Male . Adam" },
  { value: "am_echo", label: "(am_echo) - American Male . Echo" },
  { value: "am_eric", label: "(am_eric) - American Male . Eric" },
  { value: "am_fenrir", label: "(am_fenrir) - American Male . Fenrir" },
  { value: "am_liam", label: "(am_liam) - American Male . Liam" },
  { value: "am_michael", label: "(am_michael) - American Male . Michael" },
  { value: "am_onyx", label: "(am_onyx) - American Male . Onyx" },
  { value: "am_puck", label: "(am_puck) - American Male . Puck" },
  { value: "am_santa", label: "(am_santa) - American Male . Santa" },
  // ── British Female ──
  { value: "bf_alice", label: "(bf_alice) - British Female . Alice" },
  { value: "bf_emma", label: "(bf_emma) - British Female . Emma" },
  { value: "bf_lily", label: "(bf_lily) - British Female . Lily" },
  // ── British Male ──
  { value: "bm_daniel", label: "(bm_daniel) - British Male . Daniel" },
  { value: "bm_fable", label: "(bm_fable) - British Male . Fable" },
  { value: "bm_george", label: "(bm_george) - British Male . George" },
  { value: "bm_lewis", label: "(bm_lewis) - British Male . Lewis" },
  // ── Spanish (ES) ──
  { value: "ef_dora", label: "(ef_dora) - Spanish Female . Dora" },
  { value: "em_alex", label: "(em_alex) - Spanish Male . Alex" },
  { value: "em_santa", label: "(em_santa) - Spanish Male . Santa" },
  // ── French ──
  { value: "ff_siwis", label: "(ff_siwis) - French Female . Siwis" },
  // ── Hindi ──
  { value: "hf_alpha", label: "(hf_alpha) - Hindi Female . Alpha" },
  { value: "hf_beta", label: "(hf_beta) - Hindi Female . Beta" },
  { value: "hm_omega", label: "(hm_omega) - Hindi Male . Omega" },
  { value: "hm_psi", label: "(hm_psi) - Hindi Male . Psi" },
  // ── Italian ──
  { value: "if_sara", label: "(if_sara) - Italian Female . Sara" },
  { value: "im_nicola", label: "(im_nicola) - Italian Male . Nicola" },
  // ── Japanese ──
  { value: "jf_alpha", label: "(jf_alpha) - Japanese Female . Alpha" },
  { value: "jf_gongitsune", label: "(jf_gongitsune) - Japanese Female . Gongitsune" },
  { value: "jf_nezumi", label: "(jf_nezumi) - Japanese Female . Nezumi" },
  { value: "jf_tebukuro", label: "(jf_tebukuro) - Japanese Female . Tebukuro" },
  { value: "jm_kumo", label: "(jm_kumo) - Japanese Male . Kumo" },
  // ── Portuguese ──
  { value: "pf_dora", label: "(pf_dora) - Portuguese Female . Dora" },
  { value: "pm_alex", label: "(pm_alex) - Portuguese Male . Alex" },
  { value: "pm_santa", label: "(pm_santa) - Portuguese Male . Santa" },
  // ── Chinese ──
  { value: "zf_xiaobei", label: "(zf_xiaobei) - Chinese Female . Xiaobei" },
  { value: "zf_xiaoni", label: "(zf_xiaoni) - Chinese Female . Xiaoni" },
  { value: "zf_xiaoxiao", label: "(zf_xiaoxiao) - Chinese Female . Xiaoxiao" },
  { value: "zf_xiaoyi", label: "(zf_xiaoyi) - Chinese Female . Xiaoyi" },
  { value: "zm_yunjian", label: "(zm_yunjian) - Chinese Male . Yunjian" },
  { value: "zm_yunxia", label: "(zm_yunxia) - Chinese Male . Yunxia" },
  { value: "zm_yunxi", label: "(zm_yunxi) - Chinese Male . Yunxi" },
  { value: "zm_yunyang", label: "(zm_yunyang) - Chinese Male . Yunyang" },
];

// Derive a human-friendly label from a raw voice ID (e.g. "af_heart").
function labelFromId(id: string): string {
  const prefixMap: Record<string, string> = {
    af: "American Female",
    am: "American Male",
    bf: "British Female",
    bm: "British Male",
    ef: "Spanish Female",
    em: "Spanish Male",
    ff: "French Female",
    hf: "Hindi Female",
    hm: "Hindi Male",
    if: "Italian Female",
    im: "Italian Male",
    jf: "Japanese Female",
    jm: "Japanese Male",
    pf: "Portuguese Female",
    pm: "Portuguese Male",
    zf: "Chinese Female",
    zm: "Chinese Male",
  };
  const [prefix, ...rest] = id.split("_");
  const group = prefixMap[prefix] ?? prefix;
  const name = rest.join("_");
  const formatted = name.charAt(0).toUpperCase() + name.slice(1);
  return `(${id}) - ${group} . ${formatted}`;
}

// Convert a plain voice ID string to a Select option.
function voiceOption(id: string) {
  // Try to find a nicer label from the fallback list first.
  const found = KOKORO_VOICES_FALLBACK.find((v) => v.value === id);
  return found ?? { value: id, label: labelFromId(id) };
}

// Hook: fetch voices from the server, fall back to the static list.
function useVoices() {
  const [voices, setVoices] = useState<{ value: string; label: string }[]>(KOKORO_VOICES_FALLBACK);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    endpoints
      .listTtsVoices()
      .then((data) => {
        if (cancelled) return;
        if (data.voices && data.voices.length > 0) {
          setVoices(data.voices.map(voiceOption));
        }
      })
      .catch(() => {
        /* keep fallback */
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return { voices, loading };
}

const DEFAULT_PHRASE = "Hey there! I'm your personal assistant. How can I help you today?";

// Language options for the per-language voice overrides.
const LANGUAGE_OPTIONS = [
  { code: "en", label: "English" },
  { code: "ja", label: "Japanese" },
  { code: "zh", label: "Chinese" },
  { code: "ko", label: "Korean" },
  { code: "ar", label: "Arabic" },
  { code: "ru", label: "Russian" },
  { code: "hi", label: "Hindi" },
  { code: "fr", label: "French" },
  { code: "de", label: "German" },
  { code: "es", label: "Spanish" },
  { code: "pt", label: "Portuguese" },
  { code: "it", label: "Italian" },
];

const VOICE_OPTIONS_WITH_DEFAULT = (voices: { value: string; label: string }[]) => [
  { value: "", label: "-- same as default --" },
  ...voices,
];

// ─── Voice Tester ────────────────────────────────────────────────────────────

function VoiceTester({
  defaultVoice,
  voices,
}: {
  defaultVoice: string;
  voices: { value: string; label: string }[];
}) {
  const [voice, setVoice] = useState(defaultVoice);
  const [phrase, setPhrase] = useState(DEFAULT_PHRASE);
  const [playing, setPlaying] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);

  // Keep tester voice in sync when parent default changes (first render)
  // but don't override user's own selection after they interact.

  async function play() {
    if (playing) {
      audioRef.current?.pause();
      setPlaying(false);
      return;
    }
    setError(null);
    setPlaying(true);
    try {
      const buf = await endpoints.synthesizeSpeech(phrase || DEFAULT_PHRASE, voice);
      const blob = new Blob([buf], { type: "audio/mpeg" });
      const url = URL.createObjectURL(blob);
      const audio = new Audio(url);
      audioRef.current = audio;
      audio.onended = () => {
        setPlaying(false);
        URL.revokeObjectURL(url);
      };
      audio.onerror = () => {
        setPlaying(false);
        setError("Failed to play audio");
        URL.revokeObjectURL(url);
      };
      audio.play();
    } catch (e) {
      setPlaying(false);
      setError(e instanceof Error ? e.message : "TTS request failed");
    }
  }

  return (
    <Card title="Voice Tester">
      <p className="text-sm text-on-surface-variant mb-4">
        Preview any voice before saving. A default phrase is pre-filled -- just change the voice and
        hit Play.
      </p>

      <div className="space-y-4">
        <Select
          label="Voice to preview"
          value={voice}
          options={voices}
          onChange={(e) => setVoice(e.target.value)}
        />

        <TextArea
          label="Phrase"
          value={phrase}
          onChange={(e) => setPhrase(e.target.value)}
          rows={3}
          placeholder={DEFAULT_PHRASE}
        />

        <div className="flex items-center gap-3">
          <button
            onClick={play}
            className={`flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium transition-colors ${
              playing
                ? "bg-error/20 text-error hover:bg-error/30"
                : "bg-violet text-white hover:bg-violet/90"
            }`}
          >
            {playing ? (
              <>
                <span className="inline-block w-2 h-2 rounded-sm bg-error animate-pulse" />
                Stop
              </>
            ) : (
              <>
                <span className="inline-block w-0 h-0 border-y-[5px] border-y-transparent border-l-[9px] border-l-white" />
                Play
              </>
            )}
          </button>

          {error && <span className="text-sm text-error">{error}</span>}
        </div>
      </div>
    </Card>
  );
}

// ─── Main Tab ────────────────────────────────────────────────────────────────

export function VoiceTab({
  config,
  updateConfig,
  saving,
  onSave,
  saveStatus,
}: {
  config: UserConfig;
  updateConfig: UpdateConfigFn;
  saving: boolean;
  onSave: () => Promise<void>;
  saveStatus: string | null;
}) {
  const [newLang, setNewLang] = useState("");
  const { voices, loading } = useVoices();

  const langVoices = config.voice?.languageVoices ?? {};
  const defaultVoice = config.voice?.defaultVoice ?? "af_heart";

  const addedLangCodes = Object.keys(langVoices);
  const availableLanguages = LANGUAGE_OPTIONS.filter((l) => !addedLangCodes.includes(l.code));

  function setDefaultVoice(v: string) {
    updateConfig((c) => {
      c.voice = { ...c.voice, defaultVoice: v };
      return c;
    });
  }

  function setLangVoice(lang: string, voice: string) {
    updateConfig((c) => {
      const lv = { ...(c.voice?.languageVoices ?? {}) };
      if (voice === "") {
        delete lv[lang];
      } else {
        lv[lang] = voice;
      }
      c.voice = { ...c.voice, languageVoices: lv };
      return c;
    });
  }

  function addLanguage(code: string) {
    if (!code) return;
    updateConfig((c) => {
      const lv = { ...(c.voice?.languageVoices ?? {}), [code]: defaultVoice };
      c.voice = { ...c.voice, languageVoices: lv };
      return c;
    });
    setNewLang("");
  }

  function removeLang(code: string) {
    updateConfig((c) => {
      const lv = { ...(c.voice?.languageVoices ?? {}) };
      delete lv[code];
      c.voice = { ...c.voice, languageVoices: lv };
      return c;
    });
  }

  const langLabel = (code: string) => LANGUAGE_OPTIONS.find((l) => l.code === code)?.label ?? code;

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Voice"
        description="Configure the Kokoro TTS voice used for speech synthesis"
      />

      {/* Default voice */}
      <Card title="Default Voice">
        <p className="text-sm text-on-surface-variant mb-4">
          This voice is used whenever a TTS request does not specify a voice explicitly and no
          per-language override matches.
        </p>
        <Select
          label="Default voice"
          value={defaultVoice}
          options={loading ? [{ value: defaultVoice, label: "Loading voices..." }] : voices}
          onChange={(e) => setDefaultVoice(e.target.value)}
        />
      </Card>

      {/* Voice tester */}
      <VoiceTester defaultVoice={defaultVoice} voices={voices} />

      {/* Per-language overrides */}
      <Card title="Per-Language Voice Overrides">
        <p className="text-sm text-on-surface-variant mb-4">
          When the language of a TTS message is automatically detected, the matching override voice
          is used instead of the default. Useful for giving a different accent to messages in
          different languages.
        </p>

        {addedLangCodes.length > 0 && (
          <div className="space-y-3 mb-4">
            {addedLangCodes.map((code) => (
              <div key={code} className="flex items-end gap-3">
                {/* Language label badge */}
                <div className="w-28 shrink-0">
                  <p className="text-xs text-on-surface-variant uppercase tracking-wider font-mono mb-1">
                    Language
                  </p>
                  <div className="px-3 py-2 rounded-md bg-surface-low border border-outline-variant text-sm font-medium truncate">
                    {langLabel(code)}
                  </div>
                </div>

                {/* Voice selector */}
                <div className="flex-1 min-w-0">
                  <Select
                    label="Voice"
                    value={langVoices[code] ?? ""}
                    options={VOICE_OPTIONS_WITH_DEFAULT(voices)}
                    onChange={(e) => setLangVoice(code, e.target.value)}
                  />
                </div>

                {/* Remove button */}
                <button
                  onClick={() => removeLang(code)}
                  title={`Remove ${langLabel(code)} override`}
                  className="mb-0.5 w-8 h-9 flex items-center justify-center rounded-md text-on-surface-variant hover:text-error hover:bg-error/10 transition-colors leading-none"
                >
                  &times;
                </button>
              </div>
            ))}
          </div>
        )}

        {availableLanguages.length > 0 && (
          <div className="flex items-end gap-3">
            <div className="flex-1">
              <Select
                label="Add language override"
                value={newLang}
                options={[
                  { value: "", label: "-- select language --" },
                  ...availableLanguages.map((l) => ({ value: l.code, label: l.label })),
                ]}
                onChange={(e) => setNewLang(e.target.value)}
              />
            </div>
            <button
              onClick={() => addLanguage(newLang)}
              disabled={!newLang}
              className="mb-0.5 px-4 py-2 rounded-md bg-violet text-white text-sm font-medium disabled:opacity-40 hover:bg-violet/90 transition-colors"
            >
              Add
            </button>
          </div>
        )}

        {addedLangCodes.length === 0 && availableLanguages.length === LANGUAGE_OPTIONS.length && (
          <p className="text-sm text-on-surface-variant italic">
            No language overrides configured. Add one above to get started.
          </p>
        )}
      </Card>

      <SaveBar onClick={onSave} loading={saving} label="Save Voice Settings" status={saveStatus} />
    </div>
  );
}
