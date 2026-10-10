import tseslint from 'typescript-eslint';

export default tseslint.config(
  { ignores: ['dist', 'src/api/schema.gen.ts'] },
  ...tseslint.configs.strict,
);
