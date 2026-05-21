// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

// Package sdk demonstrates integrating with the Vassago memory daemon.
//
// # Quick start — use the Vassago SDK in your own agent
//
//	import "github.com/aj-nt/vassago/sdk"
//
//	func main() {
//	    ctx := context.Background()
//
//	    // One-liner: connect, register, heartbeat, subscribe.
//	    store, err := sdk.Connect(ctx, sdk.Config{
//	        Address:   "localhost:50051",
//	        AgentName: "my-agent",
//	    })
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    defer store.Close()
//
//	    // Fetch the agent's context block (memories + env).
//	    hotBlock, _ := store.HotBlock(ctx)
//	    fmt.Println("Context:", hotBlock)
//
//	    // Remember something.
//	    store.Remember(ctx, "AJ uses neovim with lazy.nvim", "project", 3)
//
//	    // Search memories.
//	    results, _ := store.Search(ctx, "neovim", 5)
//	    for _, m := range results {
//	        fmt.Println(" -", m.Content)
//	    }
//
//	    // Create a conversation session.
//	    sessionID, _ := store.CreateSession(ctx, "cli", "Quick chat")
//	    store.AppendMessages(ctx, sessionID, []client.Message{
//	        {Role: "user", Content: "What editor do I use?"},
//	    })
//	}
//
// # Responding to platform messages (Discord, Telegram, etc.)
//
//	store, _ := sdk.Connect(ctx, sdk.Config{Address: "...", AgentName: "my-agent"})
//	cm := sdk.NewConversationManager(store, myHandler, sdk.ConversationManagerConfig{})
//	cm.Run(ctx)
//
//	func myHandler(ctx context.Context, event *pb.MessageEvent, conv *sdk.Conversation) (string, error) {
//	    // event.Platform = "discord", event.SenderName = "@user"
//	    // conv.SessionID is persistent across turns for this channel.
//	    return "Hello from my-agent!", nil
//	}
package sdk
