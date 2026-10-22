import { Accordion, Accordions } from "@fumadocs/base-ui/components/accordion"
import { Step, Steps } from "@fumadocs/base-ui/components/steps"
import { Tab, Tabs } from "@fumadocs/base-ui/components/tabs"
import { TypeTable } from "@fumadocs/base-ui/components/type-table"
import defaultMdxComponents from "@fumadocs/base-ui/mdx"
import type { MDXComponents } from "mdx/types"

export function getMDXComponents(components?: MDXComponents) {
  return {
    ...defaultMdxComponents,
    Accordion,
    Accordions,
    Step,
    Steps,
    Tab,
    Tabs,
    TypeTable,
    ...components,
  } satisfies MDXComponents
}

export const useMDXComponents = getMDXComponents

declare global {
  type MDXProvidedComponents = ReturnType<typeof getMDXComponents>
}
