"use strict";
import {register} from '../nav.js';
import {addStrings} from '../i18n.js';
import {icon} from '../icons.js';
addStrings({'tab.more':'Ավելին','more.budget':'Խմբագրել բյուջեն','more.loan':'Ավելացնել վարկ'},{'tab.more':'More','more.budget':'Edit budget','more.loan':'Add loan'});
register({id:'more',icon:icon('wallet'),labelKey:'tab.more',html:'<div class="stack"><button class="card" data-go="budget-edit" data-i18n="more.budget"></button><button class="card" data-go="add" data-i18n="more.loan"></button></div>'});
