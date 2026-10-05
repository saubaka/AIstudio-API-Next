package aistudio

import (
	"context"
	"fmt"
	"slices"
)

type channelContextKey struct{}

// ContextWithChannel pins a public request to one upstream quota source.
func ContextWithChannel(ctx context.Context, channel Channel) context.Context {
	return context.WithValue(ctx, channelContextKey{}, channel)
}

func ChannelFromContext(ctx context.Context) Channel {
	channel, _ := ctx.Value(channelContextKey{}).(Channel)
	return channel
}

// PinChannelSelection prevents special capabilities and retries from silently
// escaping a channel-specific endpoint into another quota source.
func PinChannelSelection(ctx context.Context, selection AccountSelection) (AccountSelection, error) {
	channel := ChannelFromContext(ctx)
	if channel == "" {
		return selection, nil
	}
	if channel != ChannelPlayground && channel != ChannelBuild {
		return selection, fmt.Errorf("%w: unknown upstream channel", ErrInvalidArgument)
	}
	if channel == ChannelBuild && !generationChannelSelection(selection) {
		return selection, fmt.Errorf("%w: App Build does not support this Playground-only capability or file reference", ErrInvalidArgument)
	}
	selection.Channel = channel
	return selection, nil
}

// ModelsForChannel returns an independent catalog without mutating the shared
// snapshot. Build has no Playground token-counting or dedicated methods.
func ModelsForChannel(models []Model, channel Channel) []Model {
	if channel == "" {
		return models
	}
	result := make([]Model, 0, len(models))
	for _, model := range models {
		if !slices.Contains(model.Channels, string(channel)) {
			continue
		}
		model.Channels = []string{string(channel)}
		if channel == ChannelBuild {
			model.Methods = []string{"generateContent"}
		}
		result = append(result, model)
	}
	return result
}
