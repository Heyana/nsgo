import type { APIRoute } from 'astro';
import { llmsDocs, llmsFull } from '../lib/llms';

export const GET: APIRoute = async () => {
  return new Response(llmsFull(await llmsDocs()), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
};
