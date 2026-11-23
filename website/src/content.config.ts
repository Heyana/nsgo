import { docsCollection } from 'nsgo-website/content';

// The docs are docs/public/'s en/ and zh/; nsgo-website serves en/ at docs/
// and zh/ at zh/docs/.
export const collections = { docs: docsCollection() };
