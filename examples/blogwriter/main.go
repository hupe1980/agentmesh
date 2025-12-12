// Package main demonstrates a multi-agent blog writer using the Supervisor pattern.
// This example shows:
//   - Creating specialized worker agents for different blog writing tasks
//   - Using a supervisor to coordinate the workflow
//   - Generating SEO-optimized blog posts with research, writing, and review
//   - Streaming progress as the blog is created
//
// The workflow:
//   1. Keyword Generator - Creates SEO keywords from the topic
//   2. Headline Creator - Generates and evaluates headline options
//   3. Content Writer - Writes the blog post
//   4. Editor/Reviewer - Reviews and improves quality
//
// Run: OPENAI_API_KEY=sk-... go run main.go

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
)

func main() {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	ctx := context.Background()

	topic := "How AI is transforming software development productivity in 2025"

	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("🚀 AgentMesh Blog Writer")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("\nTopic: %s\n\n", topic)

	blogWriter, err := createBlogWriter()
	if err != nil {
		log.Fatalf("Failed to create blog writer: %v", err)
	}

	input := []message.Message{
		message.NewHumanMessageFromText(fmt.Sprintf(
			"Write a comprehensive blog post about: %s\n\n"+
				"Requirements:\n"+
				"- Target length: 1500-2000 words\n"+
				"- Tone: Conversational but authoritative\n"+
				"- Include statistics and examples\n"+
				"- Optimize for SEO\n"+
				"- Output in Markdown format",
			topic,
		)),
	}

	fmt.Println("📝 Starting blog generation...")
	fmt.Println(strings.Repeat("─", 60))

	results, err := graph.Collect(blogWriter.Run(ctx, input))
	if err != nil {
		log.Fatalf("Blog generation failed: %v", err)
	}

	displayResults(results)
}

func createBlogWriter() (*message.Graph, error) {
	model := openai.NewModel()

	keywordAgent, err := createKeywordAgent()
	if err != nil {
		return nil, fmt.Errorf("failed creating keyword agent: %w", err)
	}

	headlineAgent, err := createHeadlineAgent()
	if err != nil {
		return nil, fmt.Errorf("failed creating headline agent: %w", err)
	}

	writerAgent, err := createWriterAgent()
	if err != nil {
		return nil, fmt.Errorf("failed creating writer agent: %w", err)
	}

	editorAgent, err := createEditorAgent()
	if err != nil {
		return nil, fmt.Errorf("failed creating editor agent: %w", err)
	}

	return agent.NewSupervisor(
		model,
		agent.WithWorker("keywords",
			"SEO expert that generates optimized keywords for blog topics. "+
				"Analyzes the topic and produces primary keywords, secondary keywords, "+
				"and long-tail phrases for maximum search visibility.",
			keywordAgent),
		agent.WithWorker("headlines",
			"Headline specialist that creates engaging, click-worthy titles. "+
				"Generates multiple headline options in different styles (how-to, listicle, question) "+
				"and evaluates them for clickability, SEO, and clarity.",
			headlineAgent),
		agent.WithWorker("writer",
			"Expert content writer that creates comprehensive blog posts. "+
				"Writes engaging, well-structured content with hooks, examples, "+
				"and clear takeaways. Outputs in Markdown format.",
			writerAgent),
		agent.WithWorker("editor",
			"Senior editor that reviews and improves blog content. "+
				"Checks for clarity, engagement, grammar, and structure. "+
				"Provides specific suggestions and polishes the final draft.",
			editorAgent),
		agent.WithInstructions(supervisorPrompt),
		agent.WithMaxIterations(15),
		agent.WithWorkerRetries(2),
		agent.WithNodeMiddleware(progressMiddleware()),
	)
}

const supervisorPrompt = `You are a Blog Writing Supervisor coordinating a team of specialists to create high-quality blog posts.

Your workflow:
1. First, delegate to 'keywords' to generate SEO keywords from the user's topic
2. Then delegate to 'headlines' to create and select the best headline
3. Delegate to 'writer' to draft the complete blog post (include keywords and headline context)
4. Finally, delegate to 'editor' to review and polish the content

Important guidelines:
- Pass full context between agents (keywords → headlines → writer → editor)
- After the editor reviews, if the quality score is below 80/100, ask the writer to revise
- Maximum 2 revision cycles
- The final output should be a complete Markdown blog post

Always provide clear instructions when delegating to each worker.`

// progressMiddleware creates middleware that displays progress during blog generation.
// It shows which node is being executed, which tools are called, and timing info.
func progressMiddleware() graph.NodeMiddleware[message.Message] {
	return func(next graph.NodeFunc[message.Message]) graph.NodeFunc[message.Message] {
		return func(ctx context.Context, scope graph.Scope[message.Message]) (*graph.Command, error) {
			nodeName := scope.NodeName()
			start := time.Now()

			// Get context about what's happening
			messages := message.GetMessages(scope)

			switch nodeName {
			case "model":
				fmt.Printf("🤖 Thinking...\n")

			case "tool":
				// Find the last AI message to see which tools are being called
				for i := len(messages) - 1; i >= 0; i-- {
					if aiMsg, ok := messages[i].(*message.AIMessage); ok && len(aiMsg.ToolCalls) > 0 {
						for _, tc := range aiMsg.ToolCalls {
							// Parse tool name to show friendly worker name
							toolName := tc.Name
							switch {
							case strings.HasPrefix(toolName, "handoff_to_keywords"):
								fmt.Printf("🔑 Delegating to: Keywords Agent\n")
							case strings.HasPrefix(toolName, "handoff_to_headlines"):
								fmt.Printf("📰 Delegating to: Headlines Agent\n")
							case strings.HasPrefix(toolName, "handoff_to_writer"):
								fmt.Printf("✍️  Delegating to: Writer Agent\n")
							case strings.HasPrefix(toolName, "handoff_to_editor"):
								fmt.Printf("📝 Delegating to: Editor Agent\n")
							default:
								fmt.Printf("🔧 Calling tool: %s\n", toolName)
							}
						}
						break
					}
				}

			default:
				fmt.Printf("⚙️  Running: %s\n", nodeName)
			}

			result, err := next(ctx, scope)

			duration := time.Since(start)
			if err != nil {
				fmt.Printf("   ❌ Failed after %s: %v\n", duration.Round(time.Millisecond), err)
			} else if nodeName == "tool" {
				fmt.Printf("   ✅ Done (%s)\n", duration.Round(time.Millisecond))
			}

			return result, err
		}
	}
}

func createKeywordAgent() (*message.Graph, error) {
	model := openai.NewModel()

	return agent.NewReAct(
		model,
		agent.WithInstructions(`You are an SEO keyword expert specializing in content strategy.

Given a blog topic, generate:
1. ONE primary keyword (high search volume, moderate competition)
2. 5-7 secondary keywords (related terms and synonyms)
3. 3-5 long-tail keyword phrases (specific, lower competition)

Consider:
- Search intent (informational vs transactional)
- Current trends in the topic area
- Keywords that work well for Medium/blog platforms

Output your analysis in a clear, structured format that the next agent can use.`),
		agent.WithMaxIterations(3),
	)
}

func createHeadlineAgent() (*message.Graph, error) {
	model := openai.NewModel()

	return agent.NewReAct(
		model,
		agent.WithInstructions(`You are a headline specialist who has studied viral content on Medium.

Given a topic and keywords, create 5 headline options using different styles:
1. How-to/Tutorial format: "How to [Achieve X] in [Timeframe]"
2. Listicle format: "X Ways to [Achieve Goal]"
3. Question format: "Why Does [Topic] Matter for [Audience]?"
4. Provocative format: "The [Adjective] Truth About [Topic]"
5. Data-driven format: "[Number]% of [Group] Don't Know About [Topic]"

For each headline:
- Keep it 50-70 characters
- Include the primary keyword naturally
- Use power words (ultimate, essential, proven, etc.)

Then evaluate each headline on:
- Clickability (1-10)
- SEO optimization (1-10)
- Clarity (1-10)

Select the BEST headline and explain your choice.`),
		agent.WithMaxIterations(3),
	)
}

func createWriterAgent() (*message.Graph, error) {
	model := openai.NewModel()

	return agent.NewReAct(
		model,
		agent.WithInstructions(`You are an expert content writer for Medium with a track record of viral articles.

Write a comprehensive blog post following this structure:

1. **Hook** (first 2 sentences must grab attention)
2. **Introduction** (establish credibility, preview the value)
3. **Main Sections** (3-5 sections with clear H2 headers)
   - Use short paragraphs (2-3 sentences max)
   - Include bullet points for scannability
   - Add statistics, examples, or quotes where relevant
4. **Conclusion** (clear takeaway and call-to-action)

Guidelines:
- Target length: 1500-2000 words
- Write in second person ("you") for engagement
- Use conversational but authoritative tone
- Add subheadings every 300 words
- Include the provided keywords naturally (1-2% density)
- End with a question to encourage comments

Output the complete article in Markdown format with:
- # for the main title
- ## for section headers
- ### for subsections
- > for quotes
- **bold** for emphasis
- Bullet points where appropriate`),
		agent.WithMaxIterations(5),
	)
}

func createEditorAgent() (*message.Graph, error) {
	model := openai.NewModel()

	return agent.NewReAct(
		model,
		agent.WithInstructions(`You are a senior editor at a major publication reviewing a draft blog post.

Review the article for:

1. **Readability** (score 1-100)
   - Sentence length variety
   - Paragraph structure
   - Flow and transitions

2. **Engagement** (score 1-100)
   - Hook effectiveness
   - Story elements
   - Reader value

3. **SEO** (score 1-100)
   - Keyword placement
   - Header structure
   - Meta-readiness

4. **Quality** (score 1-100)
   - Grammar and spelling
   - Fact accuracy
   - Originality

Provide:
- Overall score (average of all categories)
- 3-5 specific issues with locations
- Concrete suggestions for improvement
- If score >= 80: Output "APPROVED" with the final polished version
- If score < 80: Output "NEEDS REVISION" with specific revision requests

Be constructive but rigorous.`),
		agent.WithMaxIterations(3),
	)
}

func displayResults(results []message.Message) {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))

	var lastAIMessage *message.AIMessage

	for _, msg := range results {
		if m, ok := msg.(*message.AIMessage); ok {
			lastAIMessage = m
			content := m.String()
			if strings.Contains(content, "# ") && len(content) > 1000 {
				fmt.Println("\n📄 GENERATED BLOG POST:")
				fmt.Println(strings.Repeat("=", 60))
				fmt.Println(content)
			}
		}
	}

	if lastAIMessage == nil {
		fmt.Println("❌ No blog post was generated")
		return
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("✅ Blog generation complete!")

	content := lastAIMessage.String()
	wordCount := len(strings.Fields(content))
	fmt.Printf("📊 Approximate word count: %d\n", wordCount)
}
