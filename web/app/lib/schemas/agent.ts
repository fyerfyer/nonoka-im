import { z } from "zod";

export const policySchema = z.enum(["auto", "ask", "deny"]);

export const mcpBindingSchema = z.object({
  serverId: z.number().int().positive(),
  server: z.string().optional(),
  tool: z.string().min(1),
  exposed: z.string().optional(),
  schemaHash: z.string().optional(),
  policy: policySchema.optional(),
});

export const skillBindingSchema = z.object({
  skillId: z.number().int().positive(),
  name: z.string().optional(),
  version: z.number().int().positive(),
  checksum: z.string().optional(),
});

export const agentFormSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "Name is required")
    .max(100, "Name must be at most 100 characters"),
  description: z
    .string()
    .max(4000, "Description must be at most 4000 characters"),
  instruction: z
    .string()
    .trim()
    .min(1, "Instructions are required")
    .max(20000, "Instructions must be at most 20000 characters"),
  modelServiceId: z.number().int().positive("Select a model service"),
  modelId: z.number().int().positive("Select a model"),
  tools: z.array(z.string()),
  capabilityPresets: z.array(z.string()),
  toolPolicies: z.record(z.string(), policySchema),
  maxTokens: z
    .number()
    .int()
    .min(1, "Max tokens must be positive")
    .max(128000, "Max tokens is too large"),
  mcpBindings: z.array(mcpBindingSchema),
  skills: z.array(skillBindingSchema),
});

export type AgentFormValues = z.infer<typeof agentFormSchema>;

export const shareSchema = z.object({
  kind: z.enum(["user", "group"]),
  target: z.string().min(1, "Select a target"),
  role: z.enum(["viewer", "editor"]).default("viewer"),
});


