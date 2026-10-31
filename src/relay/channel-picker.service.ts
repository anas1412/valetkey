import { Injectable } from '@nestjs/common';
import { RegistryService } from '../registry/registry.service.js';
import type { ChannelEntry } from '../common/types.js';
import type { InboundAPI } from './errors.js';

/**
 * Priority-weighted random channel selection.
 *
 * Higher priority = more traffic. A channel with priority 20 gets 2× the
 * traffic of a channel with priority 10 (all else being equal).
 */
@Injectable()
export class ChannelPickerService {
  constructor(private readonly registry: RegistryService) {}

  /**
   * Channels in the group that can serve `model` for this client API, in attempt
   * order: a weighted-random primary first, then the rest by descending priority.
   * Anthropic-format clients can only be served by anthropic channels.
   */
  candidates(groupName: string, model: string, api: InboundAPI): ChannelEntry[] {
    const eligible = this.registry.getGroupChannels(groupName).filter((e) => {
      if (api === 'anthropic' && e.channel.spec.type !== 'anthropic') return false;
      return servesModel(e, model);
    });
    if (eligible.length <= 1) return eligible;

    const primary = this.pick(eligible);
    const rest = eligible
      .filter((e) => e !== primary)
      .sort((a, b) => priority(b) - priority(a));
    return [primary, ...rest];
  }

  private pick(eligible: ChannelEntry[]): ChannelEntry {
    const totalWeight = eligible.reduce((sum, e) => sum + priority(e), 0);
    let r = Math.random() * totalWeight;
    for (const entry of eligible) {
      r -= priority(entry);
      if (r <= 0) return entry;
    }
    return eligible[eligible.length - 1];
  }
}

function priority(e: ChannelEntry): number {
  return e.channel.spec.priority ?? 10;
}

/** A channel serves a model if it has no model list, or the model is listed or mapped. */
export function servesModel(e: ChannelEntry, model: string): boolean {
  const { models, modelMapping } = e.channel.spec;
  if (!models?.length) return true;
  return models.includes(model) || (modelMapping !== undefined && model in modelMapping);
}
