<script lang="ts">
  type StatTone = 'violet' | 'green' | 'blue' | 'amber'

  interface Props {
    icon: string
    tone: StatTone
    caption: string
    value: string | number
    label: string
    muted?: boolean
  }

  let { icon, tone, caption, value, label, muted = false }: Props = $props()
</script>

<article class="stat-card" class:muted-card={muted}>
  <div class="stat-top"><span class={`stat-icon ${tone}`} aria-hidden="true">{icon}</span><span class="stat-caption">{caption}</span></div>
  <strong class="stat-value">{value}</strong>
  <span class="stat-label">{label}</span>
</article>
