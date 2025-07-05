import { faker } from '@faker-js/faker'

const methods = {
	shorten(text, prefixLen, suffixLen) {
        if (text.length <= prefixLen + suffixLen) {
            return text; // If the string is already short, return as is
        }
		let prefix = text.slice(0, prefixLen)
		let suffix = (suffixLen > 0) ? text.slice(-suffixLen) : ""
        return `${prefix}...${suffix}`;
    },
	generateRandomName() {
		return faker.word.adjective() + "-" + faker.animal.type() + "-" + Date.now().toString(36).slice(-4)
	}
}

export const textUtils = {
	data () {
		return {
		}
	},
	methods: methods
}