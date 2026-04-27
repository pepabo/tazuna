package op

type CommandBuilder struct {
	Format string
	Item   *ItemCommandBuilder
	Vault  *VaultCommandBuilder
}

func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{}
}

func (c *CommandBuilder) Build() []string {
	cmd := []string{"op"}

	if c.Format != "" {
		cmd = append(cmd, "--format", c.Format)
	}

	if c.Vault != nil {
		cmd = append(cmd, c.Vault.Build()...)
	} else if c.Item != nil {
		cmd = append(cmd, c.Item.Build()...)
	}

	return cmd
}

func (c *CommandBuilder) WithJSONFormat() *CommandBuilder {
	c.Format = "json"
	return c
}

func (c *CommandBuilder) WithVault(vault *VaultCommandBuilder) *CommandBuilder {
	c.Vault = vault
	return c
}

func (c *CommandBuilder) WithItem(item *ItemCommandBuilder) *CommandBuilder {
	c.Item = item
	return c
}

type VaultCommandBuilder struct {
	Get  *VaultGetCommandBuilder
	List *VaultListCommandBuilder
}

func NewVaultCommandBuilder() *VaultCommandBuilder {
	return &VaultCommandBuilder{}
}

func (v *VaultCommandBuilder) Build() []string {
	cmd := []string{"vault"}
	if v.List != nil {
		return append(cmd, v.List.Build()...)
	} else if v.Get != nil {
		return append(cmd, v.Get.Build()...)
	}

	return cmd
}

func (v *VaultCommandBuilder) WithList(list *VaultListCommandBuilder) *VaultCommandBuilder {
	v.List = list
	return v
}

func (v *VaultCommandBuilder) WithGet(get *VaultGetCommandBuilder) *VaultCommandBuilder {
	v.Get = get
	return v
}

type VaultGetCommandBuilder struct {
	Vault string
}

func NewVaultGetCommandBuilder() *VaultGetCommandBuilder {
	return &VaultGetCommandBuilder{}
}
func (v *VaultGetCommandBuilder) WithVault(vault string) *VaultGetCommandBuilder {
	v.Vault = vault
	return v
}
func (v *VaultGetCommandBuilder) Build() []string {
	return []string{"get", v.Vault}
}

type VaultListCommandBuilder struct{}

func NewVaultListCommandBuilder() *VaultListCommandBuilder {
	return &VaultListCommandBuilder{}
}

func (v *VaultListCommandBuilder) Build() []string {
	return []string{"list"}
}

type ItemCommandBuilder struct {
	Get    *ItemGetCommandBuilder
	Create *ItemCreateCommandBuilder
	// This method is not used in the provided code, but can be implemented if needed.
	List *ItemListCommandBuilder
}

func NewItemCommandBuilder() *ItemCommandBuilder {
	return &ItemCommandBuilder{}
}

func (i *ItemCommandBuilder) WithGet(get *ItemGetCommandBuilder) *ItemCommandBuilder {
	i.Get = get
	return i
}

func (i *ItemCommandBuilder) WithCreate(create *ItemCreateCommandBuilder) *ItemCommandBuilder {
	i.Create = create
	return i
}

func (i *ItemCommandBuilder) WithList(list *ItemListCommandBuilder) *ItemCommandBuilder {
	i.List = list
	return i
}

func (i *ItemCommandBuilder) Build() []string {
	cmd := []string{"item"}

	if i.Create != nil {
		return append(cmd, i.Create.Build()...)
	} else if i.Get != nil {
		return append(cmd, i.Get.Build()...)
	} else if i.List != nil {
		return append(cmd, i.List.Build()...)
	}

	return cmd
}

type ItemGetCommandBuilder struct {
	Vault string
	Title string
}

func NewItemGetCommandBuilder() *ItemGetCommandBuilder {
	return &ItemGetCommandBuilder{}
}

func (i *ItemGetCommandBuilder) WithVault(vault string) *ItemGetCommandBuilder {
	i.Vault = vault
	return i
}

func (i *ItemGetCommandBuilder) WithTitle(title string) *ItemGetCommandBuilder {
	i.Title = title
	return i
}

func (i *ItemGetCommandBuilder) Build() []string {
	cmd := []string{"get"}
	cmd = append(cmd, "--vault", i.Vault)
	cmd = append(cmd, i.Title)
	return cmd
}

type ItemCreateCommandBuilder struct {
	Title    string
	Vault    *string
	DryRun   bool
	Category *string
	Stdin    bool
}

func NewItemCreateCommandBuilder() *ItemCreateCommandBuilder {
	return &ItemCreateCommandBuilder{}
}

func (i *ItemCreateCommandBuilder) WithTitle(title string) *ItemCreateCommandBuilder {
	i.Title = title
	return i
}

func (i *ItemCreateCommandBuilder) WithVault(vault string) *ItemCreateCommandBuilder {
	i.Vault = &vault
	return i
}

const (
	VaultItemCategoryAPICredential = "API Credential"
)

func (i *ItemCreateCommandBuilder) WithCategory(category *string) *ItemCreateCommandBuilder {
	i.Category = category
	return i
}

func (i *ItemCreateCommandBuilder) WithDryRun(dryRun bool) *ItemCreateCommandBuilder {
	i.DryRun = dryRun
	return i
}

func (i *ItemCreateCommandBuilder) WithStdin(stdin bool) *ItemCreateCommandBuilder {
	i.Stdin = stdin
	return i
}

func (i *ItemCreateCommandBuilder) Build() []string {
	cmd := []string{"create"}

	cmd = append(cmd, "--title", i.Title)
	if i.Category != nil {
		cmd = append(cmd, "--category", *i.Category)
	}

	if i.DryRun {
		cmd = append(cmd, "--dry-run")
	}

	if i.Vault != nil {
		cmd = append(cmd, "--vault", *i.Vault)
	}

	if i.Stdin {
		cmd = append(cmd, "-")
	}

	return cmd
}

type ItemListCommandBuilder struct {
	Vault *string
}

func NewItemListCommandBuilder() *ItemListCommandBuilder {
	return &ItemListCommandBuilder{}
}

func (i *ItemListCommandBuilder) WithVault(vault string) *ItemListCommandBuilder {
	i.Vault = &vault
	return i
}

func (i *ItemListCommandBuilder) Build() []string {
	cmd := []string{"list"}

	if i.Vault != nil {
		cmd = append(cmd, "--vault", *i.Vault)
	}

	return cmd
}
